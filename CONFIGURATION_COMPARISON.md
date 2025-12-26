# ⚙️ Configuration Comparison Guide

## Quick Reference for Different Hardware

| Setting | MacBook Air M4 | 4x P100 Server | Purpose |
|---------|----------------|----------------|---------|
| **ENABLE_GPU** | `false` | `true` ✅ | GPU acceleration |
| **WORKER_CPU_LIMIT** | `1.5` | `20` | CPU cores |
| **WORKER_MEMORY_LIMIT** | `4G` | `56G` | RAM limit |
| **MAX_PARALLEL_JOBS** | `1` | `4` | Simultaneous videos |
| **MAX_PARALLEL_FFMPEG** | `1` | `12` | Parallel streams |
| **H265_PRESET** | `slower` | `medium` | Encoding preset |
| **H265_CRF** | `28` | `23` | Quality (lower=better) |

---

## 🖥️ Hardware Configurations

### MacBook Air M4 (Development/Light Use)
**Target**: Don't overheat, work quietly

```bash
# Priority: Low temperature, quiet operation
ENABLE_GPU=false
WORKER_CPU_LIMIT=1.5
WORKER_MEMORY_LIMIT=4G
MAX_PARALLEL_JOBS=1
MAX_PARALLEL_FFMPEG=1
H265_PRESET=slower         # Less CPU intensive
H265_CRF=28                # Lower bitrate
```

**Expected Performance**:
- 1080p video: 0.3-0.5x realtime (2x longer than video duration)
- Temperature: 60-75°C
- Fan noise: Minimal
- Use case: Development, testing, small batches

---

### 4x P100 GPU Server (Production)
**Target**: Maximum throughput, 24/7 operation

```bash
# Priority: Maximum performance
ENABLE_GPU=true            # ✅ CRITICAL
WORKER_CPU_LIMIT=20
WORKER_MEMORY_LIMIT=56G
MAX_PARALLEL_JOBS=4        # 1 per GPU
MAX_PARALLEL_FFMPEG=12     # 3 per GPU
H265_PRESET=medium         # GPU handles this easily
H265_CRF=23                # Higher quality
```

**Expected Performance**:
- 1080p video: 15-20x realtime (process 2hr movie in 6-8 minutes)
- GPU utilization: 80-95%
- Throughput: ~300 movies/day
- Use case: Production, high volume

---

### Other Common Configurations

#### Budget VPS (4 CPU, 8GB RAM, No GPU)
```bash
ENABLE_GPU=false
WORKER_CPU_LIMIT=4
WORKER_MEMORY_LIMIT=7G
MAX_PARALLEL_JOBS=1
MAX_PARALLEL_FFMPEG=2
H265_PRESET=fast
H265_CRF=26
```
**Performance**: 0.5-1x realtime

---

#### Workstation (RTX 4090, Ryzen 9, 32GB)
```bash
ENABLE_GPU=true
WORKER_CPU_LIMIT=24
WORKER_MEMORY_LIMIT=28G
MAX_PARALLEL_JOBS=2
MAX_PARALLEL_FFMPEG=6
H265_PRESET=medium
H265_CRF=24
```
**Performance**: 20-30x realtime (single GPU)

---

#### Cloud GPU Instance (1x T4, 8 CPU, 32GB)
```bash
ENABLE_GPU=true
WORKER_CPU_LIMIT=8
WORKER_MEMORY_LIMIT=28G
MAX_PARALLEL_JOBS=2
MAX_PARALLEL_FFMPEG=4
H265_PRESET=medium
H265_CRF=25
```
**Performance**: 10-15x realtime

---

## 📊 Throughput Calculator

### Daily Processing Capacity

**Formula**: `Daily Videos = (24 hours × GPU Speed × Parallel Jobs) / Video Length`

| Hardware | Encoding Speed | Jobs | 2-hour Movies/Day | 1-hour Videos/Day |
|----------|----------------|------|-------------------|-------------------|
| MacBook M4 | 0.5x | 1 | ~6 | ~12 |
| VPS (No GPU) | 1x | 1 | ~12 | ~24 |
| RTX 4090 | 25x | 2 | ~600 | ~1,200 |
| 1x T4 | 12x | 2 | ~288 | ~576 |
| **4x P100** | **15x** | **4** | **~360** | **~720** |

---

## 🎯 Recommended Settings by Use Case

### Development & Testing
```bash
WORKER_CPU_LIMIT=2
WORKER_MEMORY_LIMIT=4G
MAX_PARALLEL_JOBS=1
MAX_PARALLEL_FFMPEG=1
H265_PRESET=veryfast       # Fast feedback
H265_CRF=28
```

### Small Business (50-100 videos/day)
```bash
WORKER_CPU_LIMIT=8
WORKER_MEMORY_LIMIT=16G
MAX_PARALLEL_JOBS=2
MAX_PARALLEL_FFMPEG=4
ENABLE_GPU=true            # If available
H265_PRESET=medium
H265_CRF=25
```

### Enterprise (500+ videos/day)
```bash
WORKER_CPU_LIMIT=20
WORKER_MEMORY_LIMIT=56G
MAX_PARALLEL_JOBS=4
MAX_PARALLEL_FFMPEG=12
ENABLE_GPU=true            # Required
H265_PRESET=medium
H265_CRF=23
```

### Broadcast Quality (Premium)
```bash
WORKER_CPU_LIMIT=20
WORKER_MEMORY_LIMIT=56G
MAX_PARALLEL_JOBS=2        # Lower parallelism
MAX_PARALLEL_FFMPEG=6      # More time per video
ENABLE_GPU=true
H265_PRESET=slow           # Better quality
H265_CRF=20                # High bitrate
```

---

## 🔄 Migration Path

### From MacBook to Production Server

1. **Test on MacBook** (current config):
   ```bash
   cp .env.example .env
   # Keep default MacBook settings
   ```

2. **Deploy to Production**:
   ```bash
   cp .env.production .env
   # Update passwords and settings
   ```

3. **Verify GPU**:
   ```bash
   docker-compose logs worker | grep -i "gpu\|nvenc"
   ```

4. **Monitor**:
   ```bash
   nvidia-smi -l 1
   docker-compose logs -f worker
   ```

---

## 💰 Cost vs Performance

### Cloud GPU Pricing (Approximate)

| Instance Type | GPU | Cost/Hour | Videos/Day | Cost/Video |
|---------------|-----|-----------|------------|------------|
| g4dn.xlarge (AWS) | T4 | $0.52 | 300 | $0.042 |
| p3.2xlarge (AWS) | V100 | $3.06 | 400 | $0.184 |
| On-premise 4x P100 | P100 | ~$0.15* | 360 | $0.010 |

*Assuming electricity + depreciation

**Recommendation for 4x P100**: Run 24/7 for best ROI

---

## 🚦 Quick Decision Tree

```
Do you have NVIDIA GPU?
├─ YES → ENABLE_GPU=true
│   ├─ 1 GPU → MAX_PARALLEL_JOBS=2, MAX_PARALLEL_FFMPEG=4
│   ├─ 2 GPUs → MAX_PARALLEL_JOBS=2, MAX_PARALLEL_FFMPEG=6
│   └─ 4 GPUs → MAX_PARALLEL_JOBS=4, MAX_PARALLEL_FFMPEG=12
│
└─ NO → ENABLE_GPU=false
    ├─ Low CPU (<4 cores) → MAX_PARALLEL_JOBS=1, MAX_PARALLEL_FFMPEG=1
    ├─ Medium CPU (4-8) → MAX_PARALLEL_JOBS=1, MAX_PARALLEL_FFMPEG=2
    └─ High CPU (8+) → MAX_PARALLEL_JOBS=2, MAX_PARALLEL_FFMPEG=4

CPU Limit = Your total CPU threads (leave 2-4 for system)
Memory Limit = Your total RAM - 8GB (for system)
```

---

## 📝 Configuration Files Summary

| File | Purpose | Use When |
|------|---------|----------|
| `.env.example` | Template with all options | Starting new deployment |
| `.env` | Current active config | MacBook Air M4 (development) |
| `.env.production` | 4x P100 optimized | Production server |

---

## 🎓 Pro Tips

1. **Start Conservative**: Begin with lower parallelism, then increase
2. **Monitor First**: Watch GPU/CPU/RAM usage for 24 hours before scaling
3. **Test Video**: Use same test video to compare configurations
4. **Benchmark**: Measure actual throughput, not just GPU %
5. **Quality Check**: Verify output quality when changing CRF/preset

---

## 🔗 Quick Links

- [Full Production Setup Guide](./PRODUCTION_SERVER_SETUP.md)
- [Environment Variables Reference](./ENV_VARIABLES.md)
- [API Documentation](./internal/api/handlers.go)

