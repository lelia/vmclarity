#!/bin/bash

set -euo pipefail

mkdir -p /etc/vmclarity
mkdir -p /opt/vmclarity

cat << 'EOF' > /etc/vmclarity/deploy.sh
#!/bin/bash
set -euo pipefail

# Install the latest version of docker from the offical
# docker repository instead of the older version built into
# ubuntu, so that we can use docker compose v2.
#
# To install this we need to add the docker apt repo gpg key
# to the apt keyring, and then add the apt sources based on
# our version of ubuntu. Then we can finally apt install all
# the required docker components.
apt-get update
apt-get install -y ca-certificates curl gnupg
mkdir -p /etc/apt/keyrings
chmod 755 /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | sudo gpg --yes --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg
echo \
  "deb [arch="$(dpkg --print-architecture)" signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  "$(. /etc/os-release && echo "$VERSION_CODENAME")" stable" | \
  sudo tee /etc/apt/sources.list.d/docker.list > /dev/null
apt-get update
apt-get -y install docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

if [ "__DatabaseToUse__" == "Postgresql" ]; then
  # Enable and start/restart postgres
  echo "COMPOSE_PROFILES=postgres" >> /etc/vmclarity/service.env

  # Configure the VMClarity backend to use the local postgres
  # service
  echo "DATABASE_DRIVER=POSTGRES" > /etc/vmclarity/apiserver.env
  echo "DB_NAME=vmclarity" >> /etc/vmclarity/apiserver.env
  echo "DB_USER=vmclarity" >> /etc/vmclarity/apiserver.env
  echo "DB_PASS=__PostgresDBPassword__" >> /etc/vmclarity/apiserver.env
  echo "DB_HOST=postgres.service" >> /etc/vmclarity/apiserver.env
  echo "DB_PORT_NUMBER=5432" >> /etc/vmclarity/apiserver.env
elif [ "__DatabaseToUse__" == "External Postgresql" ]; then
  # Configure the VMClarity backend to use the postgres
  # database configured by the user.
  echo "DATABASE_DRIVER=POSTGRES" > /etc/vmclarity/apiserver.env
  echo "DB_NAME=__ExternalDBName__" >> /etc/vmclarity/apiserver.env
  echo "DB_USER=__ExternalDBUsername__" >> /etc/vmclarity/apiserver.env
  echo "DB_PASS=__ExternalDBPassword__" >> /etc/vmclarity/apiserver.env
  echo "DB_HOST=__ExternalDBHost__" >> /etc/vmclarity/apiserver.env
  echo "DB_PORT_NUMBER=__ExternalDBPort__" >> /etc/vmclarity/apiserver.env
elif [ "__DatabaseToUse__" == "SQLite" ]; then
  # Configure the VMClarity backend to use the SQLite DB
  # driver and configure the storage location so that it
  # persists.
  echo "DATABASE_DRIVER=LOCAL" > /etc/vmclarity/apiserver.env
  echo "LOCAL_DB_PATH=/data/vmclarity.db" >> /etc/vmclarity/apiserver.env
fi

# Replace anywhere in the config.env __CONTROLPLANE_HOST__
# with the local ipv4 IP address of the VMClarity server.
local_ip_address="$(curl -s -H Metadata:true --noproxy "*" "http://169.254.169.254/metadata/instance/network/interface/0/ipv4/ipAddress/0/privateIpAddress?api-version=2021-02-01&format=text")"
sed -i "s/__CONTROLPLANE_HOST__/${local_ip_address}/" /etc/vmclarity/orchestrator.env

# Reload the systemd daemon to ensure that the VMClarity unit
# has been detected.
systemctl daemon-reload

# Create directory required for grype-server
/usr/bin/mkdir -p /opt/grype-server
/usr/bin/chown -R 1000:1000 /opt/grype-server

# Create directory required for vmclarity apiserver
/usr/bin/mkdir -p /opt/vmclarity

# Create directory for exploit db server
/usr/bin/mkdir -p /opt/exploits

# Create directory for trivy server
/usr/bin/mkdir -p /opt/trivy-server

# Enable and start/restart VMClarity backend
systemctl enable vmclarity.service
systemctl restart vmclarity.service
EOF
chmod 744 /etc/vmclarity/deploy.sh

cat << 'EOF' > /etc/vmclarity/orchestrator.env
PROVIDER=Azure
VMCLARITY_AZURE_SUBSCRIPTION_ID=__AZURE_SUBSCRIPTION_ID__
VMCLARITY_AZURE_SCANNER_LOCATION=__AZURE_SCANNER_LOCATION__
VMCLARITY_AZURE_SCANNER_RESOURCE_GROUP=__AZURE_SCANNER_RESOURCE_GROUP__
VMCLARITY_AZURE_SCANNER_SUBNET_ID=__AZURE_SCANNER_SUBNET_ID__
VMCLARITY_AZURE_SCANNER_PUBLIC_KEY=__AZURE_SCANNER_PUBLIC_KEY__
VMCLARITY_AZURE_SCANNER_VM_SIZE=__AZURE_SCANNER_VM_SIZE__
VMCLARITY_AZURE_SCANNER_IMAGE_PUBLISHER=__AZURE_SCANNER_IMAGE_PUBLISHER__
VMCLARITY_AZURE_SCANNER_IMAGE_OFFER=__AZURE_SCANNER_IMAGE_OFFER__
VMCLARITY_AZURE_SCANNER_IMAGE_SKU=__AZURE_SCANNER_IMAGE_SKU__
VMCLARITY_AZURE_SCANNER_IMAGE_VERSION=__AZURE_SCANNER_IMAGE_VERSION__
VMCLARITY_AZURE_SCANNER_SECURITY_GROUP=__AZURE_SCANNER_SECURITY_GROUP__
VMCLARITY_AZURE_SCANNER_STORAGE_ACCOUNT_NAME=__AZURE_SCANNER_STORAGE_ACCOUNT_NAME__
VMCLARITY_AZURE_SCANNER_STORAGE_CONTAINER_NAME=__AZURE_SCANNER_STORAGE_CONTAINER_NAME__

APISERVER_HOST=apiserver
APISERVER_PORT=8888
SCANNER_CONTAINER_IMAGE=__ScannerContainerImage__
SCANNER_VMCLARITY_APISERVER_ADDRESS=http://__CONTROLPLANE_HOST__:8888
TRIVY_SERVER_ADDRESS=http://__CONTROLPLANE_HOST__:9992
GRYPE_SERVER_ADDRESS=__CONTROLPLANE_HOST__:9991
DELETE_JOB_POLICY=__AssetScanDeletePolicy__
ALTERNATIVE_FRESHCLAM_MIRROR_URL=http://__CONTROLPLANE_HOST__:1000/clamav
EOF
chmod 644 /etc/vmclarity/orchestrator.env

cat << 'EOF' > /etc/vmclarity/vmclarity.yaml
version: '3'

services:
  apiserver:
    image: __APIServerContainerImage__
    command:
      - run
      - --log-level
      - info
    ports:
      - "8888:8888"
    env_file: ./apiserver.env
    volumes:
      - type: bind
        source: /opt/vmclarity
        target: /data
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  orchestrator:
    image: __OrchestratorContainerImage__
    command:
      - run
      - --log-level
      - info
    env_file: ./orchestrator.env
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  ui:
    image: __UIContainerImage__
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  uibackend:
    image: __UIBackendContainerImage__
    command:
      - run
      - --log-level
      - info
    env_file: ./uibackend.env
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  gateway:
    image: nginx
    ports:
      - "80:80"
    configs:
      - source: gateway_config
        target: /etc/nginx/nginx.conf
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  exploit-db-server:
    image: __ExploitDBServerContainerImage__
    ports:
      - "1326:1326"
    volumes:
      - type: bind
        source: /opt/exploits
        target: /vuls
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  trivy-server:
    image: __TrivyServerContainerImage__
    command:
      - server
    ports:
      - "9992:9992"
    env_file: ./trivy-server.env
    volumes:
      - type: bind
        source: /opt/trivy-server
        target: /home/scanner/.cache
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  grype-server:
    image: __GrypeServerContainerImage__
    command:
      - run
      - --log-level
      - warning
    ports:
      - "9991:9991"
    env_file: ./grype-server.env
    volumes:
      - type: bind
        source: /opt/grype-server
        target: /opt/grype-server
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  freshclam-mirror:
    image: __FreshclamMirrorContainerImage__
    ports:
      - "1000:80"
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

  postgresql:
    image: __PostgresqlContainerImage__
    env_file: ./postgres.env
    ports:
      - "5432:5432"
    profiles:
      - postgres
    logging:
      driver: journald
    deploy:
      mode: replicated
      replicas: 1
      restart_policy:
        condition: on-failure

configs:
  gateway_config:
    file: ./gateway.conf
EOF

cat << 'EOF' > /etc/vmclarity/uibackend.env
##
## UIBackend configuration
##
# Host for the VMClarity backend server
APISERVER_HOST=apiserver
# Port number for the VMClarity backend server
APISERVER_PORT=8888
EOF
chmod 644 /etc/vmclarity/uibackend.env

cat << 'EOF' > /etc/vmclarity/service.env
# COMPOSE_PROFILES=
EOF
chmod 644 /etc/vmclarity/service.env

cat << 'EOF' > /etc/vmclarity/trivy-server.env
TRIVY_LISTEN=0.0.0.0:9992
TRIVY_CACHE_DIR=/home/scanner/.cache/trivy
EOF
chmod 644 /etc/vmclarity/trivy-server.env

cat << 'EOF' > /etc/vmclarity/grype-server.env
DB_ROOT_DIR=/opt/grype-server/db
EOF
chmod 644 /etc/vmclarity/grype-server.env

cat << 'EOF' > /etc/vmclarity/postgres.env
POSTGRESQL_USERNAME=vmclarity
POSTGRESQL_PASSWORD=__PostgresDBPassword__
POSTGRESQL_DATABASE=vmclarity
EOF
chmod 644 /etc/vmclarity/postgres.env

cat << 'EOF' > /etc/vmclarity/gateway.conf
events {
    worker_connections 1024;
}

http {
    upstream ui {
        server ui:80;
    }

    upstream uibackend {
        server uibackend:8890;
    }

    upstream apiserver {
        server apiserver:8888;
    }

    server {
        listen 80;

        location / {
            proxy_pass http://ui/;
        }

        location /ui/api/ {
            proxy_pass http://uibackend/;
        }

        location /api/ {
            proxy_pass http://apiserver/;
        }
    }
}
EOF
chmod 644 /etc/vmclarity/gateway.conf

cat << 'EOF' > /lib/systemd/system/vmclarity.service
[Unit]
Description=VmClarity
After=docker.service
Requires=docker.service

[Service]
TimeoutStartSec=0
Type=oneshot
RemainAfterExit=true
EnvironmentFile=/etc/vmclarity/service.env
ExecStart=/usr/bin/docker compose -p vmclarity -f /etc/vmclarity/vmclarity.yaml up -d --wait --remove-orphans
ExecStop=/usr/bin/docker compose -p vmclarity -f /etc/vmclarity/vmclarity.yaml down

[Install]
WantedBy=multi-user.target
EOF
chmod 644 /lib/systemd/system/vmclarity.service

/etc/vmclarity/deploy.sh
