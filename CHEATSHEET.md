# 🚀 Video Converter - Quick Reference

## 📦 Быстрый деплой (копируй и выполняй)

```bash
# Подключиться к серверу
ssh root@YOUR_SERVER_IP

# Скачать deployment script
wget https://raw.githubusercontent.com/YOUR_USERNAME/converter/main/QUICK_DEPLOY.sh
chmod +x QUICK_DEPLOY.sh

# Запустить (следовать инструкциям)
./QUICK_DEPLOY.sh
```

## ⚡ Основные команды

### Управление сервисами
```bash
cd /opt/converter

# Старт всех сервисов
docker compose up -d

# Остановить
docker compose down

# Перезапустить
docker compose restart

# Перезапустить только worker
docker compose restart worker

# Посмотреть статус
docker compose ps

# Логи в реальном времени
docker compose logs -f
docker compose logs -f worker
docker compose logs -f api
```

### Мониторинг GPU
```bash
# Текущее состояние GPU
nvidia-smi

# В реальном времени (обновление каждую секунду)
watch -n 1 nvidia-smi

# GPU в контейнере
docker exec converter-worker-1 nvidia-smi

# Использование ресурсов контейнерами
docker stats
```

### Проверка работоспособности
```bash
# API health
curl http://localhost:8080/healthz

# API ready
curl http://localhost:8080/readyz

# Версия Docker
docker --version
docker compose version

# Статус Nginx
systemctl status nginx

# Логи системы
journalctl -u docker -f
```

## 📤 Работа с видео

### Загрузить видео в MinIO
```bash
# Установить mc (если нужно)
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc && mv mc /usr/local/bin/

# Настроить подключение (один раз)
mc alias set local http://localhost:9000 MINIO_USER MINIO_PASSWORD

# Посмотреть buckets
mc ls local

# Загрузить файл
mc cp /path/to/video.mp4 local/source/input/video.mp4

# Посмотреть файлы
mc ls local/source/input/
```

### Создать задачу конвертации
```bash
# Простая задача (дефолтные настройки)
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source": {
      "type": "s3",
      "bucket": "source",
      "key": "input/video.mp4"
    }
  }'

# С указанием качества
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source": {
      "type": "s3",
      "bucket": "source",
      "key": "input/video.mp4"
    },
    "profile": {
      "qualities": ["480p", "720p", "1080p"]
    }
  }'

# Сохранить Job ID из ответа
```

### Проверить статус задачи
```bash
# Заменить JOB_ID на реальный ID
JOB_ID="your-job-id-here"

# Простой запрос
curl http://localhost:8080/v1/jobs/$JOB_ID

# Красивый вывод (требует jq)
curl -s http://localhost:8080/v1/jobs/$JOB_ID | jq

# Следить за прогрессом
watch -n 2 "curl -s http://localhost:8080/v1/jobs/$JOB_ID | jq"
```

### Получить артефакты
```bash
# Список всех артефактов
curl http://localhost:8080/v1/jobs/$JOB_ID/artifacts | jq

# Скачать результат из MinIO
mc cp local/converted/output/JOB_ID/master.m3u8 ./
```

## 🔧 Обновление приложения

```bash
cd /opt/converter

# Получить изменения из GitHub
git pull

# Пересобрать и перезапустить
docker compose down
docker compose build
docker compose up -d

# Проверить
docker compose ps
docker compose logs -f worker
```

## 🔍 Troubleshooting

### GPU не работает
```bash
# Проверить драйвер
nvidia-smi

# Проверить в Docker
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi

# Проверить в worker
docker exec converter-worker-1 nvidia-smi

# Проверить логи worker на nvenc
docker compose logs worker | grep -i nvenc

# Перезапустить Docker
systemctl restart docker
docker compose restart worker
```

### Порт занят
```bash
# Найти процесс
netstat -tulpn | grep :8080

# Убить процесс
kill -9 PID

# Или изменить порт в .env
nano /opt/converter/.env
# Изменить API_PORT=8081
docker compose restart api
```

### Нет места на диске
```bash
# Проверить место
df -h

# Очистить Docker
docker system prune -a
docker volume prune

# Очистить логи
journalctl --vacuum-time=7d

# Очистить старые образы
docker image prune -a
```

### Медленная конвертация
```bash
# Проверить GPU утилизацию (должно быть 80-95%)
nvidia-smi

# Проверить настройки в .env
cat /opt/converter/.env | grep -E "ENABLE_GPU|MAX_PARALLEL"

# Должно быть:
# ENABLE_GPU=true
# MAX_PARALLEL_JOBS=4
# MAX_PARALLEL_FFMPEG=12

# Если неправильно - исправить
nano /opt/converter/.env
docker compose restart worker
```

### Ошибка Out of Memory
```bash
# Проверить память
free -h
docker stats

# Уменьшить параллелизм
nano /opt/converter/.env
# Изменить:
# MAX_PARALLEL_JOBS=2
# MAX_PARALLEL_FFMPEG=6

docker compose restart worker
```

## 📊 Веб-интерфейсы

Замените `YOUR_IP` на IP вашего сервера:

```bash
# API Health Check
http://YOUR_IP/healthz

# MinIO Console (управление файлами)
http://YOUR_IP:9001

# Grafana (метрики и графики)
http://YOUR_IP:3000
# Логин: admin
# Пароль: смотри в /opt/converter/CREDENTIALS.txt

# Temporal UI (мониторинг workflow)
http://YOUR_IP:8088

# Prometheus (raw метрики)
http://YOUR_IP:9090
```

## 🔐 Безопасность

### Изменить пароли
```bash
# Редактировать .env
nano /opt/converter/.env

# Найти и изменить:
# POSTGRES_PASSWORD=...
# MINIO_ROOT_PASSWORD=...
# GRAFANA_ADMIN_PASSWORD=...

# Перезапустить
docker compose down
docker compose up -d
```

### Настроить Firewall
```bash
# Разрешить только необходимые порты
ufw status

# Ограничить доступ к админ панелям
ufw delete allow 9001/tcp
ufw allow from YOUR_OFFICE_IP to any port 9001

ufw delete allow 3000/tcp
ufw allow from YOUR_OFFICE_IP to any port 3000
```

### Установить SSL
```bash
# Установить Certbot
apt install -y certbot python3-certbot-nginx

# Получить сертификат (замените на ваш домен)
certbot --nginx -d yourdomain.com

# Автообновление
systemctl enable certbot.timer
```

## 📈 Производительность

### Бенчмарк
```bash
# Загрузить тестовое видео (1080p, 1 минута)
wget https://sample-videos.com/video123/mp4/1080/big_buck_bunny_1080p_30mb.mp4 -O test.mp4
mc cp test.mp4 local/source/test.mp4

# Засечь время
time curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source": {"type": "s3", "bucket": "source", "key": "test.mp4"},
    "profile": {"qualities": ["480p", "720p", "1080p"]}
  }'

# Следить за GPU
nvidia-smi -l 1
```

### Ожидаемая скорость (4x P100)
- 1080p видео: **15-20x** realtime
- 2-часовой фильм: **6-8 минут**
- Параллельно: **4 видео**
- В день: **~300-360 фильмов**

## 🔄 Backup & Restore

### Backup PostgreSQL
```bash
# Создать backup
docker exec converter-postgres-1 pg_dump -U postgres converter > backup.sql

# Сжать
gzip backup.sql

# Скопировать на локальный компьютер
scp root@SERVER_IP:/root/backup.sql.gz ./
```

### Restore PostgreSQL
```bash
# Загрузить на сервер
scp backup.sql.gz root@SERVER_IP:/root/

# Распаковать
gunzip backup.sql.gz

# Восстановить
docker exec -i converter-postgres-1 psql -U postgres converter < backup.sql
```

### Backup MinIO
```bash
# Синхронизировать на локальный компьютер
mc mirror local/source ~/minio-backup/source
mc mirror local/converted ~/minio-backup/converted
```

## 📞 Полезные ссылки

- **Полная документация**: `/opt/converter/DEPLOYMENT_GUIDE.md`
- **Production setup**: `/opt/converter/PRODUCTION_SERVER_SETUP.md`
- **Переменные окружения**: `/opt/converter/ENV_VARIABLES.md`
- **Сравнение конфигов**: `/opt/converter/CONFIGURATION_COMPARISON.md`

## 🆘 Экстренное восстановление

```bash
# Если всё сломалось - полный перезапуск
cd /opt/converter
docker compose down
docker system prune -f
docker compose up -d --build

# Проверить
docker compose ps
docker compose logs -f

# Если и это не помогло - начать заново
rm -rf /opt/converter
git clone YOUR_REPO /opt/converter
cd /opt/converter
cp .env.production .env
# Отредактировать .env
docker compose up -d --build
```

## 💡 Pro Tips

1. **Используйте tmux/screen** для длительных операций:
   ```bash
   apt install -y tmux
   tmux new -s converter
   docker compose logs -f
   # Ctrl+B, D для detach
   # tmux attach -t converter для возврата
   ```

2. **Мониторинг в одном окне**:
   ```bash
   # Установить htop и watch
   apt install -y htop

   # Разделить терминал:
   # Окно 1: nvidia-smi -l 1
   # Окно 2: docker compose logs -f worker
   # Окно 3: htop
   ```

3. **Автоматические уведомления**:
   ```bash
   # При завершении задачи отправить в Telegram/Slack
   # Настроить webhooks в коде или использовать
   # Temporal workflow signals
   ```

4. **Регулярный мониторинг**:
   ```bash
   # Добавить в crontab проверку health
   crontab -e
   # */5 * * * * curl -s http://localhost:8080/healthz || echo "API down!" | mail -s "Alert" admin@example.com
   ```

---

**Сохраните эту шпаргалку!** Она содержит все основные команды для работы с сервером.
