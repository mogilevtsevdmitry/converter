# 🚀 Quick Start - Automated Deployment

## Автоматическая установка на чистый Ubuntu сервер

### 📋 Требования
- Ubuntu Server 22.04 LTS
- 4x NVIDIA P100 GPU
- Root доступ по SSH

---

## ⚡ Вариант 1: Через скрипт (Рекомендуется)

### Первый запуск (установка драйверов)

```bash
# 1. Подключиться к серверу
ssh root@YOUR_SERVER_IP

# 2. Скачать скрипт
wget https://raw.githubusercontent.com/YOUR_USERNAME/converter/main/QUICK_DEPLOY.sh
chmod +x QUICK_DEPLOY.sh

# 3. Запустить первый раз
./QUICK_DEPLOY.sh

# Скрипт спросит:
# - GitHub repository URL (ваш репозиторий)
# - Installation directory (по умолчанию /opt/converter)

# После установки драйверов сервер ПЕРЕЗАГРУЗИТСЯ автоматически
```

### После перезагрузки

```bash
# 1. Подключиться снова
ssh root@YOUR_SERVER_IP

# 2. Запустить с флагом --skip-drivers
cd /root  # или где вы сохранили скрипт
./QUICK_DEPLOY.sh --skip-drivers

# Скрипт продолжит установку с шага 4
# Весь процесс займет 10-15 минут
```

---

## 📖 Опции скрипта

```bash
# Показать справку
./QUICK_DEPLOY.sh --help

# Пропустить установку драйверов (после перезагрузки)
./QUICK_DEPLOY.sh --skip-drivers

# Неинтерактивный режим (без запросов)
./QUICK_DEPLOY.sh --non-interactive

# Комбинация флагов
./QUICK_DEPLOY.sh --skip-drivers --non-interactive
```

---

## 🎯 Что делает скрипт?

### Первый запуск (без --skip-drivers):
1. ✅ Обновляет систему
2. ✅ Устанавливает Docker & Docker Compose
3. ✅ Устанавливает NVIDIA драйверы
4. ⏸️ **Перезагружает сервер**

### Второй запуск (с --skip-drivers):
4. ✅ Устанавливает NVIDIA Container Toolkit
5. ✅ Клонирует репозиторий
6. ✅ Настраивает .env (генерирует пароли)
7. ✅ Настраивает docker-compose для GPU
8. ✅ Устанавливает и настраивает Nginx
9. ✅ Настраивает Firewall (UFW)
10. ✅ Собирает и запускает все сервисы

---

## ✅ После установки

### Проверить статус:
```bash
# GPU
nvidia-smi

# Docker сервисы
docker compose ps

# API
curl http://localhost:8080/healthz

# GPU в worker
docker exec converter-worker-1 nvidia-smi
```

### Посмотреть credentials:
```bash
cat /opt/converter/CREDENTIALS.txt
```

Вы увидите:
- PostgreSQL пароль
- MinIO логин/пароль + URL
- Grafana пароль + URL
- Temporal UI URL
- API URL

---

## 🌐 Веб-интерфейсы

Замените `YOUR_IP` на IP сервера:

```
API:           http://YOUR_IP/v1/jobs
MinIO Console: http://YOUR_IP:9001
Grafana:       http://YOUR_IP:3000
Temporal UI:   http://YOUR_IP:8088
```

Логины и пароли в `/opt/converter/CREDENTIALS.txt`

---

## 🧪 Тестирование

```bash
# 1. Установить MinIO Client
wget https://dl.min.io/client/mc/release/linux-amd64/mc
chmod +x mc && mv mc /usr/local/bin/

# 2. Настроить (данные из CREDENTIALS.txt)
mc alias set local http://localhost:9000 MINIO_USER MINIO_PASSWORD

# 3. Загрузить тестовое видео
wget https://sample-videos.com/video123/mp4/720/big_buck_bunny_720p_1mb.mp4 -O test.mp4
mc cp test.mp4 local/source/test.mp4

# 4. Создать задачу
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
JOB_ID="paste-job-id-here"
curl http://localhost:8080/v1/jobs/$JOB_ID

# 6. Следить за GPU
nvidia-smi -l 1
```

---

## 🔄 Обновление приложения

```bash
cd /opt/converter
git pull
docker compose down
docker compose build
docker compose up -d
```

---

## 🆘 Если что-то пошло не так

### Скрипт упал на середине
```bash
# Просто запустите снова с --skip-drivers
./QUICK_DEPLOY.sh --skip-drivers
```

### GPU не обнаруживается
```bash
# Проверить драйвер
nvidia-smi

# Если не работает - переустановить драйверы
ubuntu-drivers autoinstall
reboot

# После перезагрузки
./QUICK_DEPLOY.sh --skip-drivers
```

### Docker не видит GPU
```bash
# Проверить
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi

# Если не работает - переустановить nvidia-container-toolkit
apt remove nvidia-container-toolkit
apt install nvidia-container-toolkit
nvidia-ctk runtime configure --runtime=docker
systemctl restart docker
```

---

## 📚 Дополнительная документация

После установки смотрите:

- **Полное руководство**: `/opt/converter/DEPLOYMENT_GUIDE.md`
- **Шпаргалка команд**: `/opt/converter/CHEATSHEET.md`
- **Production настройка**: `/opt/converter/PRODUCTION_SERVER_SETUP.md`
- **Все переменные**: `/opt/converter/ENV_VARIABLES.md`

---

## 💡 Типичный сценарий использования

### День 1: Установка
```bash
# Запуск 1
./QUICK_DEPLOY.sh
# → Сервер перезагрузится

# Запуск 2
./QUICK_DEPLOY.sh --skip-drivers
# → Всё готово через 10 минут
```

### День 2: Работа
```bash
# Загрузить видео
mc cp video.mp4 local/source/

# Создать задачу через API
curl -X POST http://YOUR_IP/v1/jobs ...

# Мониторить в Grafana
open http://YOUR_IP:3000
```

### День 3: Обновление
```bash
cd /opt/converter
git pull
docker compose restart worker
```

---

## 🎉 Готово!

После успешной установки:
- ✅ 4 GPU готовы к работе
- ✅ API доступен по http://YOUR_IP/v1/jobs
- ✅ Все интерфейсы настроены
- ✅ Мониторинг работает

**Производительность**: ~300-360 фильмов в день! 🚀
