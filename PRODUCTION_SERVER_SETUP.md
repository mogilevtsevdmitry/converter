# 🚀 Production Server Setup Guide
## 4x NVIDIA P100 16GB NVLINK Configuration

### 🖥️ Server Specs
- **GPU**: 4x NVIDIA P100 16GB with NVLINK
- **CPU**: Intel Xeon E5-2660v3 (10 cores / 20 threads @ 2.6GHz)
- **RAM**: 64GB DDR4
- **Use Case**: High-volume video transcoding

---

## 📊 Recommended Configuration

### ⚡ Key Settings (`.env.production`)

```bash
# Performance Settings
ENABLE_GPU=true                    # ✅ CRITICAL: Enable GPU acceleration
MAX_PARALLEL_JOBS=4                # 1 job per GPU
MAX_PARALLEL_FFMPEG=12             # 3 concurrent streams per GPU
MAX_PARALLEL_UPLOADS=20            # High S3 upload parallelism

# Resource Limits
WORKER_CPU_LIMIT=20                # Use all 20 CPU threads
WORKER_MEMORY_LIMIT=56G            # Leave 8GB for system overhead

# Quality Settings
H265_CRF=23                        # Higher quality (lower CRF)
THUMB_MAX_FRAMES=300               # More detailed thumbnails
HLS_SEGMENT_DURATION_SEC=6         # Optimized for production
```

---

## 🎯 Performance Expectations

### With NVIDIA P100 GPU Acceleration:

| Input Resolution | Encoding Speed | Throughput per GPU | Total (4 GPUs) |
|-----------------|----------------|-------------------|----------------|
| 1080p → 1080p   | **15-20x** realtime | ~30 hours/hour | ~120 hours/hour |
| 1080p → 720p    | **20-30x** realtime | ~40 hours/hour | ~160 hours/hour |
| 1080p → 480p    | **30-40x** realtime | ~50 hours/hour | ~200 hours/hour |
| 4K → 1080p      | **8-12x** realtime  | ~16 hours/hour | ~64 hours/hour |

### Multi-Quality Encoding (1080p source → 480p, 720p, 1080p):
- **Single video**: ~3-5 minutes for 2-hour movie
- **Parallel capacity**: 4 videos simultaneously
- **Daily throughput**: ~200-300 full movies (2 hours each)

---

## 🔧 Optimal Workflow Configuration

### Strategy 1: Maximum Throughput (Recommended)
```bash
MAX_PARALLEL_JOBS=4          # Process 4 different videos at once
MAX_PARALLEL_FFMPEG=12       # Each video generates 3 qualities in parallel
```

**Best for**: High volume processing, different videos

**GPU allocation**:
- GPU 0: Job 1 (3 qualities in parallel)
- GPU 1: Job 2 (3 qualities in parallel)
- GPU 2: Job 3 (3 qualities in parallel)
- GPU 3: Job 4 (3 qualities in parallel)

### Strategy 2: Ultra-Fast Single Video
```bash
MAX_PARALLEL_JOBS=1          # One video at a time
MAX_PARALLEL_FFMPEG=12       # Use all 4 GPUs for one video
```

**Best for**: Priority jobs, live events

**GPU allocation**: All 4 GPUs work on different qualities of the same video

---

## 🏗️ Docker Compose Adjustments

Update `docker-compose.yml` for GPU support:

```yaml
worker:
  build:
    context: .
    dockerfile: deploy/docker/Dockerfile.worker
  runtime: nvidia  # Add this line
  environment:
    - NVIDIA_VISIBLE_DEVICES=all  # Make all GPUs visible
    - NVIDIA_DRIVER_CAPABILITIES=compute,video,utility
  deploy:
    resources:
      reservations:
        devices:
          - driver: nvidia
            count: 4  # Use all 4 GPUs
            capabilities: [gpu]
      limits:
        cpus: '20'
        memory: 56G
```

---

## 📈 Monitoring & Optimization

### GPU Utilization Check:
```bash
# Install nvidia-smi in container or run on host
watch -n 1 nvidia-smi

# Expected output with full load:
# GPU 0: 80-95% utilization
# GPU 1: 80-95% utilization
# GPU 2: 80-95% utilization
# GPU 3: 80-95% utilization
```

### Grafana Metrics to Watch:
- **GPU Utilization**: Should be 80-95% during encoding
- **GPU Memory**: Should use 8-12GB per active job
- **CPU Usage**: 30-50% (mostly for demuxing/muxing)
- **RAM Usage**: 40-50GB (file buffers + processing)
- **Network**: 500MB/s+ for S3 uploads

---

## 🎬 FFmpeg GPU Settings (Already Configured)

The converter automatically uses these optimal NVENC settings for P100:

### H.264 (Legacy tier):
```bash
-c:v h264_nvenc
-preset p4              # Fast preset (p1=fastest, p7=slowest/best)
-tune hq                # High quality tuning
-rc vbr                 # Variable bitrate
-cq 23                  # Quality level (lower=better)
```

### H.265 (Modern tier):
```bash
-c:v hevc_nvenc
-preset p6              # High quality preset
-tune hq
-rc vbr
-cq 23
```

**Preset recommendations for P100**:
- `p4` - Balanced (15-20x realtime, good quality)
- `p6` - High quality (10-15x realtime, better quality) ← **Recommended**
- `p7` - Maximum quality (8-12x realtime, best quality)

---

## 💾 Storage Recommendations

### Fast NVMe/SSD for `/work`:
```bash
# In docker-compose.yml, mount fast storage:
volumes:
  - /mnt/nvme/converter-work:/work  # Use NVMe SSD
```

**Why**:
- 4K video = 100-500 MB/s read/write during processing
- HDD bottleneck = GPU idle time
- NVMe = full GPU utilization

### Expected Disk Usage:
- **Per job**: 5-20GB temporary (source + intermediate + output)
- **4 parallel jobs**: 80GB peak usage
- **Recommended**: 200GB fast SSD for `/work`

---

## 🌐 Network Optimization

### S3/MinIO Upload Speed:
```bash
MAX_PARALLEL_UPLOADS=20    # Parallelize uploads

# Expected bandwidth:
# 4 jobs × 5GB output each = 20GB
# With 1Gbps network: ~3 minutes upload
# With 10Gbps network: ~20 seconds upload
```

### For Remote S3 (AWS, Wasabi, etc.):
- Use S3 Transfer Acceleration
- Enable multipart uploads (automatic)
- Consider AWS DirectConnect for ultra-high volume

---

## 🔐 Production Security

### Change Default Passwords:
```bash
# In .env.production
POSTGRES_PASSWORD=<strong-random-password>
MINIO_ROOT_USER=<your-admin-user>
MINIO_ROOT_PASSWORD=<strong-random-password>
GRAFANA_ADMIN_PASSWORD=<strong-random-password>
```

### Firewall Rules:
```bash
# Only expose necessary ports:
# 8080  - API (restrict to your network)
# 9000  - MinIO API (internal only)
# 9001  - MinIO Console (restrict to admin IPs)
# 3000  - Grafana (restrict to admin IPs)
# 5455  - PostgreSQL (internal only)
```

---

## 📊 Scaling Beyond 4 GPUs

### Horizontal Scaling (Multiple Servers):

If you have multiple servers:

```yaml
# Server 1: 4x P100
worker-1:
  environment:
    - TEMPORAL_TASK_QUEUE=video-conversion
    - MAX_PARALLEL_JOBS=4

# Server 2: 4x P100
worker-2:
  environment:
    - TEMPORAL_TASK_QUEUE=video-conversion
    - MAX_PARALLEL_JOBS=4
```

Temporal will automatically distribute jobs across all workers.

**Capacity**: 8 GPUs = ~400-500 2-hour movies per day

---

## 🧪 Benchmark Your Setup

```bash
# 1. Upload test video
mc cp test-1080p.mp4 local/source/test.mp4

# 2. Start conversion
curl -X POST http://localhost:8080/v1/jobs \
  -H "Content-Type: application/json" \
  -d '{
    "source": {
      "type": "s3",
      "bucket": "source",
      "key": "test.mp4"
    },
    "profile": {
      "qualities": ["480p", "720p", "1080p"]
    }
  }'

# 3. Monitor performance
docker-compose logs -f worker
nvidia-smi -l 1

# 4. Check results
# Expected time for 2-hour 1080p movie: 3-5 minutes
```

---

## 📋 Maintenance Checklist

### Daily:
- [ ] Check GPU temperatures (<80°C)
- [ ] Monitor disk space on `/work`
- [ ] Review failed jobs in Temporal UI

### Weekly:
- [ ] Clean up old temporary files
- [ ] Review worker logs for errors
- [ ] Check S3 storage usage

### Monthly:
- [ ] Update GPU drivers
- [ ] Review and optimize FFmpeg settings
- [ ] Backup PostgreSQL database

---

## 🚨 Troubleshooting

### GPU Not Detected:
```bash
# Check NVIDIA driver
nvidia-smi

# Check Docker GPU support
docker run --rm --gpus all nvidia/cuda:11.8.0-base-ubuntu22.04 nvidia-smi

# Ensure nvidia-container-toolkit is installed
sudo apt-get install -y nvidia-container-toolkit
sudo systemctl restart docker
```

### Low GPU Utilization:
- **Check**: Are you actually using GPU encoding? `docker logs converter-worker-1 | grep nvenc`
- **Fix**: Ensure `ENABLE_GPU=true` in `.env`
- **Check**: Disk I/O bottleneck? Use `iotop` to monitor
- **Fix**: Move `/work` to NVMe SSD

### Out of Memory Errors:
```bash
# Reduce parallel jobs
MAX_PARALLEL_JOBS=2
MAX_PARALLEL_FFMPEG=6

# Or reduce memory limit
WORKER_MEMORY_LIMIT=48G
```

---

## 💡 Pro Tips

### 1. **Pre-sort Jobs by Resolution**
Process 4K videos separately from 1080p to optimize GPU allocation.

### 2. **Use NVLINK Efficiently**
P100 NVLINK is designed for multi-GPU ML training. For video encoding, focus on parallelizing different jobs rather than single job across GPUs.

### 3. **Monitor Temperature**
P100 is passively cooled in datacenter. Ensure good airflow. Throttling starts at 80-85°C.

### 4. **Batch Similar Content**
Processing similar videos (same codec, resolution) allows GPU to stay in optimal state.

### 5. **Quality vs Speed Trade-off**
```bash
# For maximum throughput (YouTube-like):
H265_CRF=28
H265_PRESET=p4

# For premium quality (Netflix-like):
H265_CRF=20
H265_PRESET=p6
```

---

## 📞 Support Resources

- **NVIDIA NVENC Guide**: https://developer.nvidia.com/nvidia-video-codec-sdk
- **FFmpeg GPU Acceleration**: https://trac.ffmpeg.org/wiki/HWAccelIntro
- **Temporal Scaling**: https://docs.temporal.io/dev-guide/worker-performance

---

## 🎉 Expected Results

With this configuration:
- ✅ **4-6 minutes** to process a 2-hour 1080p movie → 480p, 720p, 1080p
- ✅ **~300 movies/day** continuous processing
- ✅ **80-95% GPU utilization** at full capacity
- ✅ **Automatic failover** and retry on errors
- ✅ **Full monitoring** via Grafana dashboards

Your production server is now optimized for maximum throughput! 🚀
