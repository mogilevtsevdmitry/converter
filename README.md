# Video Converter Service

Сервис конвертации видео с использованием Temporal для оркестрации, FFmpeg для транскодирования и S3-совместимого хранилища.

## Возможности

- Конвертация видео в несколько качеств (480p, 720p, 1080p, 2160p, origin)
- Генерация HLS сегментов для адаптивного стриминга
- Извлечение субтитров в формате WebVTT
- Создание превью-тайлов для скруббера
- Отказоустойчивость через Temporal workflows
- Мониторинг через Prometheus/Grafana

## Требования

- Go 1.21+
- Docker и Docker Compose
- FFmpeg и FFprobe (для локального запуска)

## Структура проекта

```
├── cmd/
│   ├── api/          # HTTP API сервер
│   └── worker/       # Temporal worker
├── internal/
│   ├── api/          # HTTP handlers и роутер
│   ├── config/       # Конфигурация
│   ├── db/           # Репозитории PostgreSQL
│   ├── domain/       # Доменные модели
│   ├── ffmpeg/       # FFmpeg wrapper
│   ├── metrics/      # Prometheus метрики
│   ├── storage/s3/   # S3 клиент
│   └── temporal/     # Workflows и Activities
├── deploy/
│   ├── docker/       # Dockerfiles
│   └── prometheus/   # Конфигурация Prometheus
├── migrations/       # SQL миграции
└── docker-compose.yml
```

---

## Быстрый старт с Docker

### 1. Клонирование и настройка

```bash
git clone <repository>
cd converter

# Создайте файл конфигурации
cp .env.example .env
```

### 2. Запуск всех сервисов

```bash
docker-compose up -d
```

### 3. Проверка статуса

```bash
docker-compose ps
```

Должны быть запущены:
- `api` - HTTP API (порт 8080)
- `worker` - Temporal worker
- `postgres` - База данных (порт 5455)
- `temporal` - Temporal server (порт 7233)
- `temporal-ui` - Temporal UI (порт 8088)
- `minio` - S3-совместимое хранилище (порты 9000, 9001)
- `prometheus` - Метрики (порт 9090)
- `grafana` - Дашборды (порт 3000)

### 4. Доступ к интерфейсам

- **API:** http://localhost:8080
- **Temporal UI:** http://localhost:8088
- **MinIO Console:** http://localhost:9001 (логин: `minioadmin`, пароль: `minioadmin`)
- **Grafana:** http://localhost:3000 (логин: `admin`, пароль: `admin`)
- **Prometheus:** http://localhost:9090

---

## Локальный запуск (без Docker для worker/api)

Этот вариант удобен для разработки и отладки.

### 1. Установка зависимостей

```bash
# macOS
brew install go ffmpeg

# Ubuntu/Debian
sudo apt update
sudo apt install golang ffmpeg
```

### 2. Запуск инфраструктуры в Docker

```bash
# Запускаем только инфраструктурные сервисы
docker-compose up -d postgres temporal temporal-ui minio minio-init prometheus grafana
```

### 3. Настройка окружения

```bash
cp .env.example .env
```

Отредактируйте `.env`:

```bash
# Для локального запуска измените:
WORKDIR_ROOT=/tmp/converter-work    # Рабочая директория
API_PORT=8080                        # Порт API (если 8080 занят, измените)
```

### 4. Создание рабочей директории

```bash
mkdir -p /tmp/converter-work
```

### 5. Запуск worker

```bash
go run ./cmd/worker
```

### 6. Запуск API (в отдельном терминале)

```bash
go run ./cmd/api
```

---

## Тестирование с MinIO (локальное S3 хранилище)

### 1. Загрузка тестового видео в MinIO

**Через MinIO Console (веб-интерфейс):**

1. Откройте http://localhost:9001
2. Войдите: `minioadmin` / `minioadmin`
3. Перейдите в bucket `source`
4. Нажмите "Upload" и загрузите видеофайл

**Через командную строку (mc клиент):**

```bash
# Установка mc (MinIO Client)
# macOS
brew install minio/stable/mc

# Или через Docker
docker run --rm -it --network host minio/mc alias set myminio http://localhost:9000 minioadmin minioadmin

# Настройка alias
mc alias set myminio http://localhost:9000 minioadmin minioadmin

# Загрузка файла
mc cp /path/to/video.mp4 myminio/source/video.mp4

# Проверка
mc ls myminio/source/
```

### 2. Создание задачи конвертации

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source_url": "s3://source/video.mp4",
    "profile": {
      "qualities": ["480p", "720p"]
    }
  }'
```

**Ответ:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "PENDING",
  "created_at": "2024-01-15T10:30:00Z"
}
```

### 3. Проверка статуса задачи

```bash
# Замените {job_id} на ID из предыдущего ответа
curl http://localhost:8080/api/v1/jobs/{job_id}
```

### 4. Просмотр прогресса в Temporal UI

Откройте http://localhost:8088 и найдите workflow по ID задачи.

### 5. Просмотр результатов в MinIO

После завершения конвертации файлы появятся в bucket `converted`:

```bash
mc ls myminio/converted/{job_id}/
```

Или через веб-интерфейс MinIO Console.

---

## Тестирование с AWS S3

### 1. Настройка credentials

Отредактируйте `.env`:

```bash
S3_ENDPOINT=https://s3.amazonaws.com    # Или региональный endpoint
S3_REGION=us-east-1                      # Ваш регион
S3_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE       # Ваш Access Key
S3_SECRET_KEY=wJalrXUtnFEMI/K7MDENG...   # Ваш Secret Key
S3_BUCKET_OUTPUT=my-converted-videos     # Bucket для результатов
S3_USE_SSL=true
```

### 2. Подготовка buckets

Создайте два bucket в AWS S3:
- `source-videos` - для исходных файлов
- `converted-videos` - для результатов (должен быть публичным для чтения или настроен CloudFront)

### 3. Загрузка исходного видео

```bash
aws s3 cp /path/to/video.mp4 s3://source-videos/video.mp4
```

### 4. Создание задачи

```bash
curl -X POST http://localhost:8080/api/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source_url": "s3://source-videos/video.mp4",
    "profile": {
      "qualities": ["480p", "720p", "1080p"]
    }
  }'
```

---

## API Reference

### Создание задачи

```
POST /api/v1/jobs
```

**Request Body:**
```json
{
  "source_url": "s3://bucket/path/to/video.mp4",
  "profile": {
    "qualities": ["480p", "720p", "1080p"],
    "video_codec": "h264",
    "audio_codec": "aac",
    "preset": "medium"
  },
  "callback_url": "https://your-server.com/webhook"
}
```

**Параметры profile:**

| Параметр | Тип | По умолчанию | Описание |
|----------|-----|--------------|----------|
| `qualities` | array | Все доступные | Список качеств: `480p`, `720p`, `1080p`, `2160p`, `origin` |
| `video_codec` | string | `h264` | Видео кодек: `h264`, `h265` |
| `audio_codec` | string | `aac` | Аудио кодек: `aac`, `opus` |
| `preset` | string | `medium` | Скорость кодирования: `ultrafast`, `fast`, `medium`, `slow` |

**Примечание:** Если исходное видео имеет разрешение ниже запрошенного качества, система автоматически выберет `origin` качество (без upscaling).

### Получение статуса задачи

```
GET /api/v1/jobs/{job_id}
```

**Response:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "status": "COMPLETED",
  "progress": 100,
  "current_stage": "completed",
  "source_url": "s3://source/video.mp4",
  "created_at": "2024-01-15T10:30:00Z",
  "updated_at": "2024-01-15T10:45:00Z",
  "artifacts": [
    {
      "type": "hls_master",
      "url": "https://s3.../converted/job_id/hls/master.m3u8"
    },
    {
      "type": "thumbnail_vtt",
      "url": "https://s3.../converted/job_id/thumbs/thumbnails.vtt"
    }
  ]
}
```

**Статусы задачи:**
- `PENDING` - Ожидает выполнения
- `PROCESSING` - В процессе
- `COMPLETED` - Завершено успешно
- `FAILED` - Ошибка

### Отмена задачи

```
DELETE /api/v1/jobs/{job_id}
```

### Health Check

```
GET /health
```

### Метрики Prometheus

```
GET /metrics
```

---

## Конфигурация

### Переменные окружения

| Переменная | По умолчанию | Описание |
|------------|--------------|----------|
| `DATABASE_URL` | - | PostgreSQL connection string |
| `TEMPORAL_ADDRESS` | `localhost:7233` | Адрес Temporal server |
| `TEMPORAL_NAMESPACE` | `default` | Temporal namespace |
| `TEMPORAL_TASK_QUEUE` | `video-conversion` | Имя очереди задач |
| `S3_ENDPOINT` | - | S3 endpoint URL |
| `S3_REGION` | `us-east-1` | S3 регион |
| `S3_ACCESS_KEY` | - | S3 access key |
| `S3_SECRET_KEY` | - | S3 secret key |
| `S3_BUCKET_OUTPUT` | `converted` | Bucket для результатов |
| `WORKDIR_ROOT` | `/work` | Рабочая директория для файлов |
| `MAX_PARALLEL_JOBS` | `2` | Макс. параллельных задач |
| `MAX_PARALLEL_FFMPEG` | `4` | Макс. параллельных FFmpeg процессов |
| `FFMPEG_PATH` | `ffmpeg` | Путь к FFmpeg |
| `FFPROBE_PATH` | `ffprobe` | Путь к FFprobe |
| `FFMPEG_PROCESS_TIMEOUT` | `6h` | Таймаут FFmpeg процесса |
| `API_PORT` | `8080` | Порт HTTP API |
| `LOG_LEVEL` | `info` | Уровень логирования |

---

## Этапы обработки видео

1. **ExtractMetadata** - Скачивание файла и извлечение метаданных через FFprobe
2. **ValidateInputs** - Проверка формата, кодеков и свободного места на диске
3. **Transcode** - Конвертация в целевые качества (H.264/H.265 + AAC)
4. **ExtractSubtitles** - Извлечение субтитров в WebVTT
5. **GenerateThumbnails** - Создание превью-тайлов для скруббера
6. **SegmentHLS** - Сегментация в HLS формат
7. **UploadArtifacts** - Загрузка результатов в S3
8. **Cleanup** - Очистка временных файлов

---

## Устранение неполадок

### Worker не подключается к Temporal

```bash
# Проверьте, что Temporal запущен
docker-compose logs temporal

# Проверьте подключение
nc -zv localhost 7233
```

### Ошибка "No such file or directory" при транскодировании

Убедитесь, что запущен только один worker (либо в Docker, либо локально):

```bash
# Остановите Docker worker
docker-compose stop worker

# Запустите локально
go run ./cmd/worker
```

### Ошибка подключения к S3/MinIO

```bash
# Проверьте MinIO
curl http://localhost:9000/minio/health/live

# Проверьте buckets
mc ls myminio/
```

### Просмотр логов

```bash
# Docker
docker-compose logs -f worker
docker-compose logs -f api

# Локально - логи выводятся в консоль
```

### Очистка и перезапуск

```bash
# Остановить всё
docker-compose down

# Удалить volumes (ВНИМАНИЕ: удалит все данные!)
docker-compose down -v

# Пересобрать образы
docker-compose build --no-cache

# Запустить заново
docker-compose up -d
```

---

## Разработка

### Запуск тестов

```bash
go test ./...
```

### Сборка бинарников

```bash
go build -o bin/api ./cmd/api
go build -o bin/worker ./cmd/worker
```

### Форматирование кода

```bash
go fmt ./...
go vet ./...
```

---

## Лицензия

MIT
