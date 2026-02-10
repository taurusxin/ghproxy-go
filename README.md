# ghproxy-go

[![Release](https://img.shields.io/github/v/release/taurusxin/ghproxy-go?style=flat-square)](https://github.com/taurusxin/ghproxy-go/releases)
[![Docker](https://img.shields.io/docker/pulls/taurusxin/ghproxy-go?style=flat-square&logo=docker)](https://hub.docker.com/r/taurusxin/ghproxy-go)
![Go](https://img.shields.io/github/go-mod/go-version/taurusxin/ghproxy-go?style=flat-square&logo=go)
[![License](https://img.shields.io/github/license/taurusxin/ghproxy-go?style=flat-square)](LICENSE)

High-performance GitHub Reverse Proxy service written in Go. Supports file acceleration for Release, Archive, Blob, and Raw files, as well as Git Clone operations.

Re-implementation of [hunshcn/gh-proxy](https://github.com/hunshcn/gh-proxy) in Go with a modernized UI and improved performance.

## Features

- **🚀 High Performance**: Built with Go + Gin, supports streaming transfer for large files.
- **📦 Static Binary**: Single binary distribution, no dependencies.
- **🐳 Docker Ready**: Multi-arch Docker images (amd64/arm64) based on Alpine.
- **🖥️ Modern UI**: Clean, minimalist, dark-themed homepage for easy usage.
- **🔗 Smart Proxy**:
  - Release & Archive files
  - Blob & Raw files (automatic blob->raw conversion)
  - `git clone` support (Smart HTTP protocol)
  - `gist.github.com` support

## Demo

![Homepage](https://raw.githubusercontent.com/taurusxin/ghproxy-go/master/screenshots/homepage.png)
*(Replace this link with your actual screenshot URL if you host it)*

## Usage

### Web Interface

Visit the homepage (default `http://127.0.0.1:8972`) and paste any GitHub link to download.

### Command Line

Prefix any GitHub URL with your proxy address:

```bash
# Download Release
wget https://ghproxy.com/https://github.com/user/repo/releases/download/v1.0.0/app.zip

# Clone Repository
git clone https://ghproxy.com/https://github.com/user/repo.git
```

## Deployment

### Docker (Recommended)

```bash
docker run -d \
  --name ghproxy \
  -p 8972:8972 \
  --restart always \
  taurusxin/ghproxy-go:latest
```

### Docker Compose

```yaml
version: '3'
services:
  ghproxy:
    image: taurusxin/ghproxy-go:latest
    ports:
      - "8972:8972"
    restart: always
    environment:
      - HOST=0.0.0.0
      - PORT=8972
```

### Manual Installation

Download the latest binary from [Releases](https://github.com/taurusxin/ghproxy-go/releases).

```bash
# Linux / macOS
chmod +x ghproxy-go
./ghproxy-go

# Windows
ghproxy-go.exe
```

### Build from Source

```bash
git clone https://github.com/taurusxin/ghproxy-go.git
cd ghproxy-go
go build -o ghproxy-go .
./ghproxy-go
```

## Configuration

Configuration is done via environment variables:

| Variable | Default   | Description          |
|----------|-----------|----------------------|
| `HOST`   | `0.0.0.0` | Listening address    |
| `PORT`   | `8972`    | Listening port       |
| `GIN_MODE`| `release` | Gin framework mode   |

## License

MIT
