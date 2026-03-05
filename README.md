# SFTP Service with FUTUR API Integration

Go-based SFTP server with limited permissions using FUTUR API for authentication and data operations.

## User Authentication

The service uses **FUTUR API** for user authentication and data operations.

### Service role:
- ✅ **Authenticates** users via FUTUR API
- ✅ **Provides** secure SFTP access
- ✅ **Integrates** with Next.js FUTUR API endpoints

## User Permissions

Users have the following **limited permissions**:

### ✅ Allowed operations:
1. **Root directory listing** (`/`) - Shows only `in` and `Hinnat` folders
2. **Navigation to directories**:
   - `/in/` - incoming files directory (**write only**)
   - `/Hinnat/` - price list directory (read/write)
3. **File operations**:
   - `/in/` → Upload files via FUTUR API
   - `/Hinnat/` → Read price lists via FUTUR API
4. **Directory listing** for `/in/` and `/Hinnat/`

### ❌ Forbidden operations:
- **No delete permissions** (files or directories)
- **No rename permissions**
- **No access to other directories** except `/in/` and `/Hinnat/`
- **No write permissions to root directory** (`/`)
- **No directory deletion permissions**

## Architecture

```
SFTP Client → SFTP Server → FUTUR API (Next.js)
```

### API Integration

1. **/in/ directory** → FUTUR Order API
   - File uploads sent to `/api/futur/order` endpoint
   - Files processed by Next.js application

2. **/Hinnat/ directory** → FUTUR Pricelist API
   - Price lists fetched from `/api/futur/pricelist` endpoint
   - User-specific content via API authentication

## API Endpoints

### Authentication
- **POST** `/api/futur/login` - User authentication
- Headers: `X-ApiKey: {api-key}`
- Body: `{"username": "user", "password": "pass"}`

### Price Lists  
- **GET** `/api/futur/pricelist` - Get user's price lists
- Headers: `X-ApiKey: {api-key}`

### Orders
- **POST** `/api/futur/order` - Upload order files  
- Headers: `X-ApiKey: {api-key}`
- Body: File content as multipart form data

## Quick Setup Guide

### Local Development

1. **Install dependencies**
```bash
go mod tidy
```

2. **Configure environment**
```bash
cp .env.example .env
# Edit .env file - set FUTUR_API_URL to point to your Next.js application
```

3. **Generate SSH host key**
```bash
ssh-keygen -t rsa -b 2048 -f hostkey -N ""
```

4. **Start the service**
```bash
go run main.go
# or
go build -o sftp-service.exe .
./sftp-service.exe
```

### AWS Production Deployment

1. **Build and push Docker image**
```powershell
# Windows PowerShell
.\build-and-push.ps1

# Linux/Mac
chmod +x build-and-push.sh
./build-and-push.sh
```

2. **Deploy infrastructure**
```bash
cd cdk
cdk deploy
```

3. **Complete deployment (build + deploy)**
```powershell
# Windows - does both steps above
.\deploy.ps1
```

### 5. Test connection
```bash
sftp -P 2222 username@localhost
```

## SFTP Commands Usage

### Allowed commands:
```bash
# List root directory (shows only 'in' and 'Hinnat')
ls

# Navigate to directories
cd in
cd Hinnat

# List directory contents
ls

# Upload file
put local_file.txt

# Download file (from Hinnat only)
get remote_file.txt
```

### Forbidden commands (return error):
```bash
# Delete file - NOT ALLOWED
rm file.txt

# Rename - NOT ALLOWED  
rename old.txt new.txt

# Delete directory - NOT ALLOWED
rmdir directory

# Write to root directory - NOT ALLOWED
put file.txt /

# Navigate to other directories - NOT ALLOWED
cd /tmp
cd /home
```

## Security Features

- **SSH host key** automatically generated and stored
- **API authentication** via FUTUR API
- **User isolation** - each user sees only their own data
- **Path validation** - prevents access to forbidden directories
- **Operation restrictions** - only reading, writing and listing allowed
- **TLS encryption** for all SFTP connections

## Logging and Monitoring

The service logs all:
- Authentication attempts
- File operations (successful and denied)
- Connection opens and closes
- Access permission errors

```bash
# View service logs
tail -f sftp-service.log

# Or if running in foreground
go run main.go
```

## Testing Permissions

```bash
# Connect via SFTP
sftp -P 2222 testuser@localhost

# Test allowed operations
ls                    # Output: in  Hinnat
cd in                 # Success
put test.txt          # Success
ls                    # Shows files
cd /Hinnat            # Success
get pricelist.csv     # Success

# Test forbidden operations
rm test.txt           # Error: access denied
cd /tmp               # Error: access denied
put test.txt /        # Error: write not allowed
```

## Troubleshooting

### Common issues:

1. **Connection denied**: Check port 2222 and firewall
2. **Authentication failed**: Verify username and password with FUTUR API
3. **File upload failed**: Check FUTUR_API_URL configuration and API availability
4. **"Access denied" errors**: Normal behavior for restricted paths

### Log checking:
```bash
# Service logs
tail -f sftp-service.log

# Check if service is running
ps aux | grep sftp-service
```

## ☁️ AWS Deployment (CDK)

The project includes ready AWS CDK configuration for production deployment.

### DNS Configuration

The SFTP service is accessible via custom domain:

- **SFTP endpoint**: `futur.salhydro.fi` (port 22)
- **DNS type**: CNAME record pointing to NLB (`sftp-service-nlb-a6d5cd0da283ddea.elb.eu-north-1.amazonaws.com`)
- **Region**: `eu-north-1` (Stockholm)

```bash
# Connect to production SFTP
sftp futur.salhydro.fi
```

### AWS Infrastructure:
- **ECS Fargate** - Container execution
- **Network Load Balancer** - SFTP traffic distribution
- **VPC** - Network infrastructure

### Quick AWS deployment guide:

```bash
# Go to CDK directory
cd cdk

# Install dependencies
npm install

# Bootstrap CDK (first time only)
cdk bootstrap

# Deploy the stack
cdk deploy
```

See complete instructions: [`cdk/README.md`](cdk/README.md)

### AWS costs (estimate):
- **~$50/month** for basic usage (EU-West-1)
- Includes: Fargate, Load Balancer, NAT Gateway
- External: Data transfer costs for FUTUR API calls
