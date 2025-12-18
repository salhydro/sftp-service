import * as cdk from 'aws-cdk-lib';
import * as ecs from 'aws-cdk-lib/aws-ecs';
import * as ec2 from 'aws-cdk-lib/aws-ec2';
import * as elbv2 from 'aws-cdk-lib/aws-elasticloadbalancingv2';
import * as efs from 'aws-cdk-lib/aws-efs';
import * as logs from 'aws-cdk-lib/aws-logs';
import * as ecr from 'aws-cdk-lib/aws-ecr';
import { Construct } from 'constructs';

export class SftpServiceStack extends cdk.Stack {
  constructor(scope: Construct, id: string, props?: cdk.StackProps) {
    super(scope, id, props);

    // VPC for the infrastructure
    const vpc = new ec2.Vpc(this, 'SftpServiceVpc', {
      maxAzs: 2,
      natGateways: 1,
      subnetConfiguration: [
        {
          cidrMask: 24,
          name: 'PublicSubnet',
          subnetType: ec2.SubnetType.PUBLIC,
        },
        {
          cidrMask: 24,
          name: 'PrivateSubnet',
          subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS,
        },

      ],
    });

    // ECS Cluster for Fargate
    const cluster = new ecs.Cluster(this, 'SftpCluster', {
      vpc,
      clusterName: 'sftp-service-cluster',
    });

    // CloudWatch Log Group
    const logGroup = new logs.LogGroup(this, 'SftpLogGroup', {
      logGroupName: '/ecs/sftp-service',
      retention: logs.RetentionDays.ONE_MONTH,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // Security Group for EFS
    const efsSecurityGroup = new ec2.SecurityGroup(this, 'EfsSecurityGroup', {
      vpc,
      description: 'Security group for EFS',
      allowAllOutbound: false,
    });

    // EFS File System for persistent host key storage
    const fileSystem = new efs.FileSystem(this, 'SftpEfsFileSystem', {
      vpc,
      lifecyclePolicy: efs.LifecyclePolicy.AFTER_30_DAYS,
      performanceMode: efs.PerformanceMode.GENERAL_PURPOSE,
      throughputMode: efs.ThroughputMode.BURSTING,
      securityGroup: efsSecurityGroup,
      removalPolicy: cdk.RemovalPolicy.DESTROY,
    });

    // EFS Access Point for /data directory
    const accessPoint = new efs.AccessPoint(this, 'SftpDataAccessPoint', {
      fileSystem,
      path: '/data',
      createAcl: {
        ownerUid: '0',
        ownerGid: '0',
        permissions: '755',
      },
      posixUser: {
        uid: '0',
        gid: '0',
      },
    });

    // Get reference to existing ECR repository
    const ecrRepository = ecr.Repository.fromRepositoryName(
      this, 
      'SftpEcrRepository', 
      'futur-sftp-service'
    );

    // Task Definition (CDK will automatically create execution role with ECR permissions)
    const taskDefinition = new ecs.FargateTaskDefinition(this, 'SftpTaskDefinition', {
      memoryLimitMiB: 1024,
      cpu: 512,
    });

    // Add EFS volume to task definition
    taskDefinition.addVolume({
      name: 'sftp-data-volume',
      efsVolumeConfiguration: {
        fileSystemId: fileSystem.fileSystemId,
        authorizationConfig: {
          accessPointId: accessPoint.accessPointId,
        },
        transitEncryption: 'ENABLED',
      },
    });

    // Use pre-built Docker image from ECR (this automatically grants pull permissions)
    const dockerImage = ecs.ContainerImage.fromEcrRepository(ecrRepository, 'latest');

    // Container Definition
    const container = taskDefinition.addContainer('SftpContainer', {
      image: dockerImage,
      logging: ecs.LogDrivers.awsLogs({
        streamPrefix: 'sftp-service',
        logGroup,
      }),
      environment: {
        FUTUR_API_URL: 'https://api.salhydro.fi/api/futur',
        SFTP_HOST_KEY_PATH: '/data/host_key',
        SFTP_PORT: '22',
      },
    });

    // Add EFS mount point to container
    container.addMountPoints({
      sourceVolume: 'sftp-data-volume',
      containerPath: '/data',
      readOnly: false,
    });

    // Add port mapping for SFTP
    container.addPortMappings({
      containerPort: 22,
      protocol: ecs.Protocol.TCP,
    });

    // Security Group for SFTP service
    const sftpSecurityGroup = new ec2.SecurityGroup(this, 'SftpSecurityGroup', {
      vpc,
      description: 'Security group for SFTP service',
      allowAllOutbound: true,
    });

    // Allow SFTP traffic (port 22) only from within VPC (NLB will forward traffic)
    sftpSecurityGroup.addIngressRule(
      ec2.Peer.ipv4(vpc.vpcCidrBlock),
      ec2.Port.tcp(22),
      'SFTP access from within VPC'
    );

    // Allow NFS traffic from SFTP containers to EFS
    efsSecurityGroup.addIngressRule(
      sftpSecurityGroup,
      ec2.Port.tcp(2049),
      'NFS access from SFTP containers'
    );

    // Fargate Service (in private subnet behind NLB)
    const service = new ecs.FargateService(this, 'SftpService', {
      cluster,
      taskDefinition,
      desiredCount: 1,
      assignPublicIp: false, // No public IP needed behind NLB
      vpcSubnets: {
        subnetType: ec2.SubnetType.PRIVATE_WITH_EGRESS, // Private subnet with NAT gateway
      },
      securityGroups: [sftpSecurityGroup],
      platformVersion: ecs.FargatePlatformVersion.LATEST,
    });

    // Network Load Balancer for SFTP (TCP traffic)
    const nlb = new elbv2.NetworkLoadBalancer(this, 'SftpLoadBalancer', {
      vpc,
      internetFacing: true,
      loadBalancerName: 'sftp-service-nlb',
    });

    // Target Group for SFTP service
    const targetGroup = new elbv2.NetworkTargetGroup(this, 'SftpTargetGroup', {
      port: 22,
      protocol: elbv2.Protocol.TCP,
      vpc,
      targetType: elbv2.TargetType.IP,
      healthCheck: {
        protocol: elbv2.Protocol.TCP,
        port: '22',
      },
    });

    // Add Fargate service to target group
    service.attachToNetworkTargetGroup(targetGroup);

    // Listener for NLB
    nlb.addListener('SftpListener', {
      port: 22, // Standard SFTP port
      protocol: elbv2.Protocol.TCP,
      defaultTargetGroups: [targetGroup],
    });

    // Outputs
    new cdk.CfnOutput(this, 'SftpEndpoint', {
      value: nlb.loadBalancerDnsName,
      description: 'SFTP server endpoint (connect on port 22)',
    });


  }
}