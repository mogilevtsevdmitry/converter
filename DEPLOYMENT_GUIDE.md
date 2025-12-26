# 🚀 Deployment Guide - Ubuntu Server Setup

Пошаговое руководство по развертыванию video converter на чистом Ubuntu Server с 4x NVIDIA P100.

---

## 📋 Предварительные требования

- Ubuntu Server 22.04 LTS (чистая установка)
- Доступ по SSH с root правами
- Публичный/приватный IP адрес
- GitHub репозиторий с проектом

---

## 🔧 Шаг 1: Подключение к серверу

```bash
# С вашего локального компьютера
ssh root@YOUR_SERVER_IP

# Или с пользователем
ssh username@YOUR_SERVER_IP
sudo -i
```

---

## 📦 Шаг 2: Обновление системы

```bash
# Обновить списки пакетов
apt update

# Обновить установленные пакеты
apt upgrade -y

# Установить базовые утилиты
apt install -y \
    curl \
    wget \
    git \
    vim \
    htop \
    net-tools \
    ca-certificates \
    gnupg \
    lsb-release
```

---

## 🐳 Шаг 3: Установка Docker

```bash
# Удалить старые версии Docker (если есть)
apt remove -y docker docker-engine docker.io containerd runc

# Добавить официальный GPG ключ Docker
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/ubuntu/gpg | gpg --dearmor -o /etc/apt/keyrings/docker.gpg
chmod a+r /etc/apt/keyrings/docker.gpg

# Добавить Docker репозиторий
echo \
  "deb [arch=$(dpkg --print-architecture) signed-by=/etc/apt/keyrings/docker.gpg] https://download.docker.com/linux/ubuntu \
  $(lsb_release -cs) stable" | tee /etc/apt/sources.list.d/docker.list > /dev/null

# Обновить списки пакетов
apt update

# Установить Docker Engine, Docker Compose
apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin

# Проверить установку
docker --version
docker compose version

# Добавить пользователя в группу docker (опционально, если не root)
# usermod -aG docker $USER
# newgrp docker

# Включить автозапуск Docker
systemctl enable docker
systemctl start docker

# Проверить статус
systemctl status docker
```

**Ожидаемый вывод:**
```
Docker version 24.x.x
Docker Compose version v2.x.x
● docker.service - Docker Application Container Engine
   Active: active (running)
```

---

## 🎮 Шаг 4: Установка NVIDIA драйверов и CUDA

```bash
# Проверить наличие GPU
lspci | grep -i nvidia

# Установить драйверы NVIDIA
ubuntu-drivers devices

# Установить рекомендуемый драйвер
ubuntu-drivers autoinstall

# Или установить конкретную версию (для P100 рекомендуется 525+)
apt install -y nvidia-driver-525

# Перезагрузить сервер
reboot

# После перезагрузки - проверить установку
nvidia-smi
```

**Ожидаемый вывод `nvidia-smi`:**
```
+-----------------------------------------------------------------------------+
| NVIDIA-SMI 525.xx       Driver Version: 525.xx       CUDA Version: 12.0    |
|-------------------------------+----------------------+----------------------+
| GPU  Name        Persistence-M| Bus-Id        Disp.A | Volatile Uncorr. ECC |
|   0  Tesla P100-PCIE...  Off  | 00000000:00:04.0 Off |                    0 |
|   1  Tesla P100-PCIE...  Off  | 00000000:00:05.0 Off |                    0 |
|   2  Tesla P100-PCIE...  Off  | 00000000:00:06.0 Off |                    0 |
|   3  Tesla P100-PCIE...  Off  | 00000000:00:07.0 Off |                    0 |
+-----------------------------------------------------------------------------+
```

---

## 🔌 Шаг 5: Установка NVIDIA Container Toolkit

```bash
# Добавить GPG ключ
distribution=$(. /etc/os-release;echo $ID$VERSION_ID)
curl -fsSL https://nvidia.github.io/libnvidia-container/gpgkey | gpg --dearmor -o /usr/share/keyrings/nvidia-container-toolkit-keyring.gpg

# Добавить репозиторий
curl -s -L https://nvidia.github.io/libnvidia-container/$distribution/libnvidia-container.list | \
  sed 's#deb https://#deb [signed-by=/usr/share/keyrings/nvidia-container-toolkit-keyring.gpg] https://#g' | \
  tee /etc/apt/sources.list.d/nvidia-container-toolkit.list

# Установить
apt update
apt install -y nvidia-container-toolkit

# Настроить Docker для использования NVIDIA runtime
nvidia-ctk runtime configure --runtime=docker
systemctl restart docker

# Проверить GPU в Docker
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi
```

**Если команда выше показывает GPU - всё настроено правильно!**

---

## 📂 Шаг 6: Клонирование репозитория

```bash
# Создать директорию для приложения
mkdir -p /opt/converter
cd /opt/converter

# Клонировать репозиторий (замените на ваш URL)
git clone https://github.com/YOUR_USERNAME/converter.git .

# Или если приватный репозиторий:
# 1. Сгенерировать SSH ключ на сервере
ssh-keygen -t ed25519 -C "server@yourcompany.com" -f ~/.ssh/id_ed25519 -N ""

# 2. Показать публичный ключ
cat ~/.ssh/id_ed25519.pub
# Скопировать вывод и добавить в GitHub Settings → SSH Keys

# 3. Клонировать через SSH
git clone git@github.com:YOUR_USERNAME/converter.git .

# Проверить содержимое
ls -la
```

**Ожидаемое содержимое:**
```
.env.example
.env.production
docker-compose.yml
internal/
deploy/
...
```

---

## ⚙️ Шаг 7: Настройка переменных окружения

```bash
# Скопировать production конфиг
cp .env.production .env

# Отредактировать конфиг
nano .env
```

**Обязательно измените следующие параметры:**

```bash
# БЕЗОПАСНОСТЬ - ИЗМЕНИТЕ ЭТИ ПАРОЛИ!
POSTGRES_PASSWORD=YOUR_STRONG_PASSWORD_HERE
MINIO_ROOT_USER=YOUR_MINIO_USERNAME
MINIO_ROOT_PASSWORD=YOUR_STRONG_MINIO_PASSWORD
GRAFANA_ADMIN_PASSWORD=YOUR_GRAFANA_PASSWORD

# Порты (если нужно изменить)
API_PORT=8080
POSTGRES_PORT=5455
MINIO_PORT=9000
MINIO_CONSOLE_PORT=9001

# GPU (ОБЯЗАТЕЛЬНО true для P100)
ENABLE_GPU=true

# Производительность (оптимизировано для 4x P100)
MAX_PARALLEL_JOBS=4
MAX_PARALLEL_FFMPEG=12
WORKER_CPU_LIMIT=20
WORKER_MEMORY_LIMIT=56G

# Качество
H265_PRESET=medium
H265_CRF=23
```

**Сохранить и выйти:** `Ctrl+O`, `Enter`, `Ctrl+X`

---

## 🔧 Шаг 8: Обновление docker-compose.yml для GPU

```bash
# Отредактировать docker-compose.yml
nano docker-compose.yml
```

**Найти секцию `worker:` и добавить поддержку GPU:**

```yaml
  worker:
    build:
      context: .
      dockerfile: deploy/docker/Dockerfile.worker
    runtime: nvidia  # ← ДОБАВИТЬ ЭТУ СТРОКУ
    ports:
      - "${WORKER_PORT:-9091}:9090"
    environment:
      - NVIDIA_VISIBLE_DEVICES=all  # ← ДОБАВИТЬ
      - NVIDIA_DRIVER_CAPABILITIES=compute,video,utility  # ← ДОБАВИТЬ
      # ... остальные переменные ...
    deploy:
      resources:
        reservations:  # ← ДОБАВИТЬ ЭТУ СЕКЦИЮ
          devices:
            - driver: nvidia
              count: 4  # Количество GPU
              capabilities: [gpu]
        limits:
          cpus: '${WORKER_CPU_LIMIT:-20}'
          memory: ${WORKER_MEMORY_LIMIT:-56G}
```

**Сохранить и выйти**

---

## 🌐 Шаг 9: Установка и настройка Nginx

```bash
# Установить Nginx
apt install -y nginx

# Создать конфигурацию для converter
nano /etc/nginx/sites-available/converter
```

**Вставить следующую конфигурацию:**

```nginx
# API Server
server {
    listen 80;
    server_name YOUR_SERVER_IP;  # Замените на ваш IP или домен

    # Увеличить лимиты для загрузки больших файлов
    client_max_body_size 10G;
    client_body_timeout 300s;
    proxy_read_timeout 300s;

    # API
    location /v1/ {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_cache_bypass $http_upgrade;
    }

    # Health checks
    location /healthz {
        proxy_pass http://localhost:8080;
    }

    location /readyz {
        proxy_pass http://localhost:8080;
    }

    # Metrics
    location /metrics {
        proxy_pass http://localhost:8080;
    }
}

# MinIO Console
server {
    listen 9001;
    server_name YOUR_SERVER_IP;

    location / {
        proxy_pass http://localhost:9001;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}

# Grafana
server {
    listen 3000;
    server_name YOUR_SERVER_IP;

    location / {
        proxy_pass http://localhost:3000;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}

# Temporal UI
server {
    listen 8088;
    server_name YOUR_SERVER_IP;

    location / {
        proxy_pass http://localhost:8088;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection 'upgrade';
        proxy_set_header Host $host;
        proxy_cache_bypass $http_upgrade;
    }
}
```

**Активировать конфигурацию:**

```bash
# Создать символическую ссылку
ln -s /etc/nginx/sites-available/converter /etc/nginx/sites-enabled/

# Удалить дефолтную конфигурацию
rm /etc/nginx/sites-enabled/default

# Проверить конфигурацию
nginx -t

# Перезагрузить Nginx
systemctl restart nginx
systemctl enable nginx

# Проверить статус
systemctl status nginx
```

---

## 🚀 Шаг 10: Запуск приложения

```bash
# Перейти в директорию проекта
cd /opt/converter

# Собрать образы (первый раз займет 5-10 минут)
docker compose build

# Запустить все сервисы
docker compose up -d

# Проверить статус
docker compose ps
```

**Ожидаемый вывод:**

```
NAME                       STATUS    PORTS
converter-api-1            running   0.0.0.0:8080->8080/tcp
converter-worker-1         running   0.0.0.0:9091->9090/tcp
converter-postgres-1       running   0.0.0.0:5455->5432/tcp
converter-temporal-1       running   0.0.0.0:7233->7233/tcp
converter-temporal-ui-1    running   0.0.0.0:8088->8080/tcp
converter-minio-1          running   0.0.0.0:9000-9001->9000-9001/tcp
converter-prometheus-1     running   0.0.0.0:9090->9090/tcp
converter-grafana-1        running   0.0.0.0:3000->3000/tcp
```

---

## ✅ Шаг 11: Проверка работоспособности

```bash
# 1. Проверить логи worker (должен видеть GPU)
docker compose logs worker | grep -i "gpu\|nvenc"

# 2. Проверить API
curl http://localhost:8080/healthz

# 3. Проверить GPU в контейнере
docker exec converter-worker-1 nvidia-smi

# 4. Проверить все сервисы
curl http://localhost:8080/healthz
curl http://localhost:8080/readyz
curl http://localhost:9090/metrics  # Prometheus
```

**Ожидаемые ответы:**
```json
{"status":"healthy","database":"healthy","s3":"healthy"}
{"status":"ready"}
```

---

## 🔥 Шаг 12: Настройка Firewall

```bash
# Установить UFW (если не установлен)
apt install -y ufw

# Разрешить SSH (ВАЖНО! Иначе потеряете доступ)
ufw allow 22/tcp

# Разрешить HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp

# Разрешить API и сервисы
ufw allow 8080/tcp   # API
ufw allow 9000/tcp   # MinIO API
ufw allow 9001/tcp   # MinIO Console
ufw allow 3000/tcp   # Grafana
ufw allow 8088/tcp   # Temporal UI

# Включить firewall
ufw enable

# Проверить статус
ufw status
```

**Для production рекомендуется ограничить доступ:**

```bash
# Разрешить доступ только с определенных IP
ufw allow from YOUR_OFFICE_IP to any port 3000  # Grafana
ufw allow from YOUR_OFFICE_IP to any port 9001  # MinIO Console
ufw allow from YOUR_OFFICE_IP to any port 8088  # Temporal UI

# API оставить открытым или настроить через Nginx + Basic Auth
```

---

## 📊 Шаг 13: Мониторинг

### Проверить GPU:
```bash
# В реальном времени
watch -n 1 nvidia-smi

# Логи worker
docker compose logs -f worker
```

### Веб-интерфейсы:

Откройте в браузере (замените YOUR_SERVER_IP):

- **API Health**: `http://YOUR_SERVER_IP/healthz`
- **MinIO Console**: `http://YOUR_SERVER_IP:9001` (логин из .env)
- **Grafana**: `http://YOUR_SERVER_IP:3000` (admin / пароль из .env)
- **Temporal UI**: `http://YOUR_SERVER_IP:8088`
- **Prometheus**: `http://YOUR_SERVER_IP:9090`

---

## 🧪 Шаг 14: Тестирование

```bash
# 1. Установить MinIO Client на сервере
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc
mv mc /usr/local/bin/

# 2. Настроить подключение к MinIO
mc alias set local http://localhost:9000 YOUR_MINIO_USER YOUR_MINIO_PASSWORD

# 3. Загрузить тестовое видео
# Скачать небольшое видео для теста
wget https://sample-videos.com/video123/mp4/720/big_buck_bunny_720p_1mb.mp4 -O test.mp4

# Загрузить в MinIO
mc cp test.mp4 local/source/test.mp4

# 4. Создать задачу конвертации
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source": {
      "type": "s3",
      "bucket": "source",
      "key": "test.mp4"
    },
    "profile": {
      "qualities": ["480p", "720p"]
    }
  }'

# 5. Получить Job ID из ответа и проверить статус
JOB_ID="PASTE_JOB_ID_HERE"
curl http://localhost:8080/v1/jobs/$JOB_ID

# 6. Следить за прогрессом
watch -n 2 "curl -s http://localhost:8080/v1/jobs/$JOB_ID | jq"

# 7. Проверить GPU загрузку во время конвертации
nvidia-smi -l 1
```

---

## 🔄 Шаг 15: Автозапуск при перезагрузке

```bash
# Docker уже настроен на автозапуск
# Проверить:
systemctl is-enabled docker

# Создать systemd service для автозапуска docker-compose (опционально)
cat > /etc/systemd/system/converter.service <<'EOF'
[Unit]
Description=Video Converter Service
Requires=docker.service
After=docker.service

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=/opt/converter
ExecStart=/usr/bin/docker compose up -d
ExecStop=/usr/bin/docker compose down
TimeoutStartSec=0

[Install]
WantedBy=multi-user.target
EOF

# Включить сервис
systemctl enable converter.service
systemctl start converter.service

# Проверить статус
systemctl status converter.service
```

---

## 🔐 Шаг 16: Безопасность (Production)

### 1. Настроить SSL (Let's Encrypt)

```bash
# Установить Certbot
apt install -y certbot python3-certbot-nginx

# Получить SSL сертификат (замените на ваш домен)
certbot --nginx -d yourdomain.com

# Автоматическое обновление
systemctl enable certbot.timer
```

### 2. Настроить Basic Auth для админ панелей

```bash
# Установить утилиту
apt install -y apache2-utils

# Создать файл с паролями
htpasswd -c /etc/nginx/.htpasswd admin

# Обновить Nginx конфиг для Grafana
nano /etc/nginx/sites-available/converter

# Добавить в location / для Grafana:
auth_basic "Restricted Access";
auth_basic_user_file /etc/nginx/.htpasswd;

# Перезагрузить Nginx
nginx -t && systemctl reload nginx
```

### 3. Ограничить доступ к портам

```bash
# В docker-compose.yml изменить порты на localhost only:
# Было: - "5455:5432"
# Стало: - "127.0.0.1:5455:5432"

# Применить изменения
cd /opt/converter
docker compose down
docker compose up -d
```

---

## 📝 Полезные команды

### Управление сервисами:
```bash
# Посмотреть логи
docker compose logs -f
docker compose logs -f worker
docker compose logs -f api

# Перезапустить сервисы
docker compose restart
docker compose restart worker

# Остановить всё
docker compose down

# Запустить снова
docker compose up -d

# Пересобрать и запустить
docker compose up -d --build

# Посмотреть статус
docker compose ps

# Использование ресурсов
docker stats
```

### Обновление приложения:
```bash
cd /opt/converter

# Получить обновления из GitHub
git pull

# Пересобрать и перезапустить
docker compose down
docker compose build
docker compose up -d
```

### Очистка:
```bash
# Очистить старые образы
docker system prune -a

# Очистить volumes (ВНИМАНИЕ: удалит данные!)
docker compose down -v

# Очистить логи
journalctl --vacuum-time=7d
```

---

## 🚨 Troubleshooting

### Проблема: GPU не обнаруживается

```bash
# Проверить драйвер
nvidia-smi

# Проверить Docker GPU support
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi

# Перезапустить Docker
systemctl restart docker
docker compose restart worker
```

### Проблема: Порты заняты

```bash
# Найти процесс на порту
netstat -tulpn | grep :8080

# Убить процесс
kill -9 PID

# Или изменить порты в .env
nano .env
```

### Проблема: Out of Memory

```bash
# Проверить использование памяти
free -h
docker stats

# Уменьшить параллелизм в .env
MAX_PARALLEL_JOBS=2
MAX_PARALLEL_FFMPEG=6

# Перезапустить
docker compose restart worker
```

---

## ✅ Чеклист финальной проверки

- [ ] `nvidia-smi` показывает 4 GPU
- [ ] `docker compose ps` - все сервисы running
- [ ] `curl http://localhost:8080/healthz` возвращает healthy
- [ ] `docker compose logs worker | grep nvenc` показывает GPU encoder
- [ ] MinIO Console открывается по http://IP:9001
- [ ] Grafana открывается по http://IP:3000
- [ ] Тестовое видео успешно конвертируется
- [ ] UFW настроен и активен
- [ ] SSL сертификат установлен (для production)
- [ ] Пароли изменены с дефолтных

---

## 🎉 Готово!

Ваш video converter развернут и готов к работе!

### Доступные URL:

- **API**: `http://YOUR_SERVER_IP/v1/jobs`
- **MinIO Console**: `http://YOUR_SERVER_IP:9001`
- **Grafana**: `http://YOUR_SERVER_IP:3000`
- **Temporal UI**: `http://YOUR_SERVER_IP:8088`

### Следующие шаги:

1. Загрузите реальные видео в MinIO
2. Настройте мониторинг в Grafana
3. Интегрируйте API в ваше приложение
4. Настройте регулярные бэкапы PostgreSQL

### Поддержка:

- Документация API: `/opt/converter/internal/api/handlers.go`
- Production setup: `/opt/converter/PRODUCTION_SERVER_SETUP.md`
- Конфигурация: `/opt/converter/ENV_VARIABLES.md`

🚀 **Happy encoding!**
