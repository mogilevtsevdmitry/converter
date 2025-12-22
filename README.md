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
| `HLS_ENABLE_ENCRYPTION` | `false` | Включить AES-128 шифрование HLS |
| `HLS_KEY_URL` | - | URL для получения ключа дешифровки |
| `DRM_ENABLED` | `false` | Включить DRM защиту |
| `DRM_PROVIDER` | `widevine` | DRM провайдер: widevine, fairplay, playready, all |
| `SHAKA_PACKAGER_PATH` | `packager` | Путь к Shaka Packager |
| `DRM_KEY_SERVER_URL` | - | URL сервера лицензий |
| `DRM_WIDEVINE_KEY_ID` | - | Widevine Key ID (hex) |
| `DRM_WIDEVINE_KEY` | - | Widevine ключ шифрования (hex) |
| `DRM_FAIRPLAY_KEY_URL` | - | FairPlay URL ключа |
| `DRM_PLAYREADY_LA_URL` | - | PlayReady License Acquisition URL |

---

## Шифрование HLS (AES-128)

Сервис поддерживает шифрование HLS сегментов с помощью AES-128-CBC.

### Включение шифрования

```bash
# В .env или docker-compose.yml
HLS_ENABLE_ENCRYPTION=true
HLS_KEY_URL=https://your-server.com/keys/{job_id}/encryption.key
```

### Как это работает

1. При создании HLS сегментов генерируется случайный 16-байтный ключ и IV
2. Все `.ts` сегменты шифруются с помощью AES-128-CBC
3. В плейлист добавляется тег `#EXT-X-KEY` с URL ключа
4. Ключ загружается в S3 вместе с сегментами

### Варианты доставки ключей

**Вариант 1: Публичный ключ (минимальная защита)**
```bash
HLS_KEY_URL=
# Ключ будет доступен по относительному пути encryption.key
```

**Вариант 2: Защищённый сервер ключей**
```bash
HLS_KEY_URL=https://api.example.com/keys/{job_id}/encryption.key
# Ваш сервер должен проверять авторизацию перед выдачей ключа
```

### Пример плейлиста с шифрованием

```m3u8
#EXTM3U
#EXT-X-VERSION:3
#EXT-X-KEY:METHOD=AES-128,URI="https://api.example.com/keys/abc123/encryption.key",IV=0x1234567890abcdef1234567890abcdef
#EXTINF:4.000,
720p_00000.ts
#EXTINF:4.000,
720p_00001.ts
...
```

### Важно

- Для реальной защиты контента ключи должны выдаваться через защищённый API с авторизацией
- AES-128 — базовый уровень защиты. Для коммерческого контента рекомендуется DRM (Widevine, FairPlay, PlayReady)
- Ключ хранится в S3 рядом с сегментами. Для защиты настройте приватный доступ к файлу ключа

---

## DRM защита (Widevine, FairPlay, PlayReady)

Сервис поддерживает полноценную DRM защиту контента с использованием Shaka Packager.

### Поддерживаемые провайдеры

| Провайдер | Платформы | Схема шифрования |
|-----------|-----------|------------------|
| **Widevine** | Android, Chrome, Firefox, Edge | CENC (AES-CTR) |
| **FairPlay** | iOS, Safari, Apple TV | CBCS (AES-CBC) |
| **PlayReady** | Windows, Xbox, Smart TV | CENC (AES-CTR) |

### Требования

1. **Shaka Packager** — должен быть установлен и доступен по пути `SHAKA_PACKAGER_PATH`

```bash
# Установка на macOS
brew install shaka-packager

# Установка на Ubuntu
wget https://github.com/shaka-project/shaka-packager/releases/latest/download/packager-linux-x64
chmod +x packager-linux-x64
sudo mv packager-linux-x64 /usr/local/bin/packager
```

2. **DRM ключи** — получите от вашего DRM провайдера (Widevine, BuyDRM, PallyCon и т.д.)

### Конфигурация

```bash
# Основные настройки
DRM_ENABLED=true
DRM_PROVIDER=widevine  # widevine, fairplay, playready, all
SHAKA_PACKAGER_PATH=packager

# Widevine
DRM_WIDEVINE_KEY_ID=your_key_id_hex    # 32 символа hex (16 байт)
DRM_WIDEVINE_KEY=your_key_hex          # 32 символа hex (16 байт)
DRM_WIDEVINE_PSSH=base64_pssh_box      # Опционально: PSSH box

# FairPlay (только HLS)
DRM_FAIRPLAY_KEY_URL=https://your-server.com/fairplay/key
DRM_FAIRPLAY_CERT_PATH=/path/to/certificate.cer
DRM_FAIRPLAY_IV=random_iv_hex          # 32 символа hex

# PlayReady (только DASH)
DRM_PLAYREADY_KEY_ID=your_key_id_hex
DRM_PLAYREADY_KEY=your_key_hex
DRM_PLAYREADY_LA_URL=https://license.example.com/playready
```

### Провайдер "all" (мульти-DRM)

При `DRM_PROVIDER=all` генерируются манифесты, совместимые со всеми провайдерами:

- **HLS** с FairPlay для Apple устройств
- **DASH** с Widevine и PlayReady для остальных

```bash
DRM_PROVIDER=all
# Настройте ключи для каждого провайдера
```

### API для получения ключей

Для тестирования и разработки доступны эндпоинты:

```bash
# Получить информацию о DRM ключе (JSON)
GET /v1/keys/{job_id}

# Ответ:
{
  "keyId": "abcd1234...",
  "provider": "widevine",
  "laUrl": "https://license.example.com/widevine"
}

# Получить сырой ключ (для HLS AES-128)
GET /v1/keys/{job_id}/encryption.key
# Возвращает 16 байт бинарного ключа
```

### Выходные файлы

При включённом DRM генерируются:

```
{job_id}/
├── master.m3u8         # HLS мастер-плейлист
├── manifest.mpd        # DASH манифест
├── 480p_video.mp4      # Зашифрованные видео сегменты
├── 720p_video.mp4
├── 1080p_video.mp4
├── audio.mp4           # Зашифрованный аудио
├── 480p_video.m3u8     # HLS плейлисты по качествам
├── 720p_video.m3u8
├── 1080p_video.m3u8
└── audio.m3u8
```

### Пример интеграции с плеером

**Shaka Player (Widevine/PlayReady):**

```javascript
const player = new shaka.Player(videoElement);

player.configure({
  drm: {
    servers: {
      'com.widevine.alpha': 'https://license.example.com/widevine',
      'com.microsoft.playready': 'https://license.example.com/playready'
    }
  }
});

await player.load('https://cdn.example.com/videos/{job_id}/manifest.mpd');
```

**HLS.js с FairPlay:**

```javascript
const hls = new Hls({
  emeEnabled: true,
  drmSystems: {
    'com.apple.fps.1_0': {
      licenseUrl: 'https://license.example.com/fairplay',
      serverCertificateUrl: 'https://license.example.com/fairplay/cert'
    }
  }
});

hls.loadSource('https://cdn.example.com/videos/{job_id}/master.m3u8');
```

### Важно

- DRM ключи должны храниться в защищённом месте (env variables, secrets manager)
- Для production используйте настоящий DRM провайдер (Widevine, BuyDRM, PallyCon)
- Тестовые ключи из API эндпоинта `/v1/keys/` предназначены только для разработки
- При `DRM_KEY_SERVER_URL=""` API вернёт сам ключ — не делайте этого в production!

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
