#!/bin/bash

echo "🔍 NOMAD-GO DIAGNOSTIC REPORT"
echo "======================================"
echo ""

# 1. Docker статус
echo "📦 Docker Status:"
docker --version
docker-compose --version
echo ""

# 2. Docker daemon
echo "🖥️ Docker Daemon:"
docker ps -a
echo ""

# 3. Текущая папка
echo "📁 Current Directory:"
pwd
echo ""

# 4. Файлы в проекте
echo "📄 Project Files:"
ls -la | head -20
echo ""

# 5. Docker Compose статус
echo "⚙️ Docker Compose Status:"
docker-compose ps
echo ""

# 6. Логи
echo "📋 Recent Logs (last 50 lines):"
docker-compose logs --tail=50
echo ""

# 7. Занятые порты
echo "🔌 Open Ports:"
if command -v netstat &> /dev/null; then
  netstat -tlnp 2>/dev/null | grep -E ":(80|3000|3001|3004|3005|3333|9090|5432|5672)"
elif command -v ss &> /dev/null; then
  ss -tlnp 2>/dev/null | grep -E ":(80|3000|3001|3004|3005|3333|9090|5432|5672)"
else
  echo "netstat/ss not available"
fi
echo ""

# 8. Docker network
echo "🌐 Docker Networks:"
docker network ls
echo ""

# 9. Проверка образов
echo "🏗️ Docker Images:"
docker images | grep nomad
echo ""

# 10. Проверка volumes
echo "💾 Docker Volumes:"
docker volume ls | grep nomad
echo ""

# 11. Проверка nginx
echo "🔒 Nginx Container:"
docker-compose ps nginx
docker exec nomad-go-nginx-1 nginx -t 2>&1 || echo "Nginx check failed"
echo ""

# 12. Проверка connectivity
echo "🔗 Connectivity Tests:"
echo "  Testing localhost:80 (nginx)..."
curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" http://localhost:80/ || echo "Failed to connect"

echo "  Testing localhost:3333 (grafana)..."
curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" http://localhost:3333/ || echo "Failed to connect"

echo "  Testing localhost:9090 (prometheus)..."
curl -s -o /dev/null -w "HTTP Status: %{http_code}\n" http://localhost:9090/ || echo "Failed to connect"

echo ""
echo "======================================"
echo "✅ Diagnostic complete!"
