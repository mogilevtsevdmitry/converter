#!/bin/bash

# ============================================
# Quick Deployment Script for Ubuntu Server
# Video Converter with 4x NVIDIA P100
# ============================================

set -e  # Exit on error

echo "🚀 Starting Video Converter Deployment..."
echo ""

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# ============================================
# Parse Arguments
# ============================================
SKIP_DRIVERS=false
SKIP_PROMPTS=false

for arg in "$@"; do
    case $arg in
        --skip-drivers)
            SKIP_DRIVERS=true
            shift
            ;;
        --non-interactive)
            SKIP_PROMPTS=true
            shift
            ;;
        --help)
            echo "Usage: $0 [OPTIONS]"
            echo ""
            echo "Options:"
            echo "  --skip-drivers        Skip NVIDIA driver installation (use after reboot)"
            echo "  --non-interactive     Use defaults, no prompts"
            echo "  --help                Show this help message"
            exit 0
            ;;
    esac
done

# ============================================
# Configuration
# ============================================
INSTALL_DIR=${INSTALL_DIR:-/opt/converter}

# If skipping drivers, repo should already be cloned
if [ "$SKIP_DRIVERS" = true ]; then
    if [ ! -d "$INSTALL_DIR" ]; then
        echo -e "${RED}ERROR: Directory $INSTALL_DIR not found!${NC}"
        echo -e "${RED}Repository should already be cloned when using --skip-drivers${NC}"
        exit 1
    fi
    echo -e "${GREEN}✓ Using existing installation at: ${INSTALL_DIR}${NC}"
    REPO_URL=""
elif [ "$SKIP_PROMPTS" = false ]; then
    read -p "Enter your GitHub repository URL (e.g., git@github.com:user/converter.git): " REPO_URL
    read -p "Enter installation directory [${INSTALL_DIR}]: " USER_DIR
    INSTALL_DIR=${USER_DIR:-$INSTALL_DIR}
else
    REPO_URL=${REPO_URL:-""}
fi

if [ "$SKIP_DRIVERS" = false ] && [ "$SKIP_PROMPTS" = false ]; then
    echo ""
    echo -e "${YELLOW}Repository: ${REPO_URL}${NC}"
    echo -e "${YELLOW}Install to: ${INSTALL_DIR}${NC}"
    echo ""
    read -p "Continue? (y/n): " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

# ============================================
# Step 1: System Update
# ============================================
echo ""
echo -e "${GREEN}[1/10] Updating system...${NC}"
apt update && apt upgrade -y
apt install -y curl wget git vim htop net-tools ca-certificates gnupg lsb-release

# ============================================
# Step 2: Install Docker
# ============================================
echo ""
echo -e "${GREEN}[2/10] Installing Docker...${NC}"

# Remove old versions
apt remove -y docker docker-engine docker.io containerd runc 2>/dev/null || true

# Add Docker GPG key
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

# Add Docker repository
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

# Install Docker
apt update
apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Enable Docker
systemctl enable docker
systemctl start docker

echo -e "${GREEN}✓ Docker installed: $(docker --version)${NC}"

# ============================================
# Step 3: Install NVIDIA Drivers
# ============================================
echo ""
echo -e "${GREEN}[3/10] Installing NVIDIA drivers...${NC}"

# Check for NVIDIA GPU
if ! lspci | grep -i nvidia > /dev/null; then
    echo -e "${RED}ERROR: No NVIDIA GPU detected!${NC}"
    exit 1
fi

# Check if drivers already installed or should skip
if [ "$SKIP_DRIVERS" = true ]; then
    echo -e "${YELLOW}Skipping driver installation (--skip-drivers flag)${NC}"

    # Verify drivers are working
    if nvidia-smi &> /dev/null; then
        echo -e "${GREEN}✓ NVIDIA drivers working${NC}"
        nvidia-smi --query-gpu=name --format=csv,noheader
    else
        echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        echo -e "${RED}ERROR: nvidia-smi failed!${NC}"
        echo -e "${RED}Drivers are installed but not loaded.${NC}"
        echo ""
        echo -e "${YELLOW}This usually means:${NC}"
        echo -e "  1. Drivers were just installed but system not rebooted"
        echo -e "  2. Kernel module not loaded"
        echo ""
        echo -e "${YELLOW}Solution:${NC}"
        echo -e "  ${GREEN}reboot${NC}  # Reboot now"
        echo -e "  ${GREEN}# Then run: ./QUICK_DEPLOY.sh --skip-drivers${NC}"
        echo -e "${RED}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
        exit 1
    fi
elif nvidia-smi &> /dev/null; then
    echo -e "${GREEN}✓ NVIDIA drivers already installed and working${NC}"
    nvidia-smi --query-gpu=name --format=csv,noheader
else
    echo "Installing NVIDIA drivers..."
    ubuntu-drivers autoinstall

    echo ""
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}NVIDIA drivers installed. System will reboot in 10 seconds...${NC}"
    echo -e "${YELLOW}After reboot, run this script again with --skip-drivers flag:${NC}"
    echo -e "${GREEN}./QUICK_DEPLOY.sh --skip-drivers${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    sleep 10
    reboot
fi

# ============================================
# Step 4: Install NVIDIA Container Toolkit
# ============================================
echo ""
echo -e "${GREEN}[4/10] Installing NVIDIA Container Toolkit...${NC}"

distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

curl -s -L https://nvidia.github.io/libnvidia-container/$distribution/libnvidia-container.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

apt update
apt install -y nvidia-container-toolkit

nvidia-ctk runtime configure --runtime=docker
systemctl restart docker

echo -e "${GREEN}✓ NVIDIA Container Toolkit installed${NC}"

# Test GPU in Docker
if docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi > /dev/null 2>&1; then
    echo -e "${GREEN}✓ GPU detected in Docker${NC}"
else
    echo -e "${RED}ERROR: GPU not accessible in Docker${NC}"
    exit 1
fi

# ============================================
# Step 5: Clone Repository
# ============================================
echo ""
echo -e "${GREEN}[5/10] Setting up repository...${NC}"

if [ "$SKIP_DRIVERS" = true ]; then
    echo -e "${YELLOW}Using existing repository at ${INSTALL_DIR}${NC}"
    cd $INSTALL_DIR
else
    mkdir -p $(dirname $INSTALL_DIR)
    if [ -d "$INSTALL_DIR" ]; then
        echo -e "${YELLOW}Directory exists. Pulling latest changes...${NC}"
        cd $INSTALL_DIR
        git pull
    else
        git clone $REPO_URL $INSTALL_DIR
        cd $INSTALL_DIR
    fi
    echo -e "${GREEN}✓ Repository ready at ${INSTALL_DIR}${NC}"
fi

# ============================================
# Step 6: Configure Environment
# ============================================
echo ""
echo -e "${GREEN}[6/10] Configuring environment...${NC}"

if [ ! -f .env ]; then
    cp .env.production .env
    echo -e "${YELLOW}Created .env from .env.production${NC}"

    # Generate random passwords
    POSTGRES_PWD=$(openssl rand -base64 32)
    MINIO_USER="admin"
    MINIO_PWD=$(openssl rand -base64 32)
    GRAFANA_PWD=$(openssl rand -base64 32)

    # Update .env
    sed -i "s/CHANGE_ME_PRODUCTION_PASSWORD/${POSTGRES_PWD}/g" .env
    sed -i "s/CHANGE_ME_MINIO_USER/${MINIO_USER}/g" .env
    sed -i "s/CHANGE_ME_MINIO_PASSWORD/${MINIO_PWD}/g" .env
    sed -i "s/CHANGE_ME_GRAFANA_PASSWORD/${GRAFANA_PWD}/g" .env

    # Save credentials
    cat > ${INSTALL_DIR}/CREDENTIALS.txt <<EOF
# IMPORTANT: Keep this file secure!

PostgreSQL:
  User: postgres
  Password: ${POSTGRES_PWD}
  Port: 5455

MinIO:
  User: ${MINIO_USER}
  Password: ${MINIO_PWD}
  Console: http://$(hostname -I | awk '{print $1}'):9001

Grafana:
  User: admin
  Password: ${GRAFANA_PWD}
  URL: http://$(hostname -I | awk '{print $1}'):3000

Temporal UI:
  URL: http://$(hostname -I | awk '{print $1}'):8088

API:
  URL: http://$(hostname -I | awk '{print $1}'):8080
EOF

    chmod 600 ${INSTALL_DIR}/CREDENTIALS.txt

    echo -e "${GREEN}✓ Environment configured${NC}"
    echo -e "${YELLOW}Credentials saved to: ${INSTALL_DIR}/CREDENTIALS.txt${NC}"
else
    echo -e "${YELLOW}.env already exists, skipping${NC}"
fi

# ============================================
# Step 7: Update docker-compose for GPU
# ============================================
echo ""
echo -e "${GREEN}[7/10] Configuring docker-compose for GPU...${NC}"

# Backup original
cp docker-compose.yml docker-compose.yml.backup

# Add GPU support to worker if not already present
if ! grep -q "runtime: nvidia" docker-compose.yml; then
    echo -e "${YELLOW}Adding GPU support to docker-compose.yml...${NC}"
    echo -e "${RED}IMPORTANT: You need to manually add GPU configuration to docker-compose.yml${NC}"
    echo -e "${RED}See DEPLOYMENT_GUIDE.md Step 8 for details${NC}"
fi

# ============================================
# Step 8: Install Nginx
# ============================================
echo ""
echo -e "${GREEN}[8/10] Installing and configuring Nginx...${NC}"

apt install -y nginx

# Create Nginx config
SERVER_IP=$(hostname -I | awk '{print $1}')

cat > /etc/nginx/sites-available/converter <<EOF
server {
    listen 80;
    server_name ${SERVER_IP};
    client_max_body_size 10G;
    client_body_timeout 300s;
    proxy_read_timeout 300s;

    location /v1/ {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
    }

    location /healthz {
        proxy_pass http://localhost:8080;
    }

    location /readyz {
        proxy_pass http://localhost:8080;
    }
}
EOF

ln -sf /etc/nginx/sites-available/converter /etc/nginx/sites-enabled/
rm -f /etc/nginx/sites-enabled/default

nginx -t && systemctl restart nginx
systemctl enable nginx

echo -e "${GREEN}✓ Nginx configured${NC}"

# ============================================
# Step 9: Configure Firewall
# ============================================
echo ""
echo -e "${GREEN}[9/10] Configuring firewall...${NC}"

apt install -y ufw

# Allow SSH first!
ufw allow 22/tcp

# Allow services
ufw allow 80/tcp
ufw allow 443/tcp
ufw allow 8080/tcp
ufw allow 9000/tcp
ufw allow 9001/tcp
ufw allow 3000/tcp
ufw allow 8088/tcp

# Enable UFW
echo "y" | ufw enable

echo -e "${GREEN}✓ Firewall configured${NC}"

# ============================================
# Step 10: Build and Start
# ============================================
echo ""
echo -e "${GREEN}[10/10] Building and starting services...${NC}"

cd $INSTALL_DIR

echo "Building Docker images (this may take 5-10 minutes)..."
docker compose build

echo "Starting services..."
docker compose up -d

# Wait for services to start
echo "Waiting for services to start..."
sleep 30

# Check status
docker compose ps

# ============================================
# Verification
# ============================================
echo ""
echo -e "${GREEN}==================================${NC}"
echo -e "${GREEN}Deployment Complete!${NC}"
echo -e "${GREEN}==================================${NC}"
echo ""

# Check API
if curl -s http://localhost:8080/healthz | grep -q "healthy"; then
    echo -e "${GREEN}✓ API is healthy${NC}"
else
    echo -e "${RED}✗ API health check failed${NC}"
fi

# Check GPU
if docker exec converter-worker-1 nvidia-smi > /dev/null 2>&1; then
    echo -e "${GREEN}✓ GPU accessible in worker container${NC}"
else
    echo -e "${RED}✗ GPU not accessible in worker${NC}"
fi

# Show access information
echo ""
echo -e "${YELLOW}Access Information:${NC}"
echo -e "API:          http://${SERVER_IP}/v1/jobs"
echo -e "MinIO:        http://${SERVER_IP}:9001"
echo -e "Grafana:      http://${SERVER_IP}:3000"
echo -e "Temporal UI:  http://${SERVER_IP}:8088"
echo ""
echo -e "${YELLOW}Credentials saved in:${NC} ${INSTALL_DIR}/CREDENTIALS.txt"
echo ""
echo -e "${YELLOW}Next steps:${NC}"
echo "1. Review credentials: cat ${INSTALL_DIR}/CREDENTIALS.txt"
echo "2. Test GPU: nvidia-smi"
echo "3. Monitor logs: docker compose logs -f worker"
echo "4. Upload test video and create conversion job"
echo ""
echo -e "${GREEN}Happy encoding! 🎬${NC}"
