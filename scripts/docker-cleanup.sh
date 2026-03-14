#!/bin/bash
set -e

echo "========== Docker Weekly Maintenance $(date) =========="

echo "Disk usage before cleanup:"
docker system df

echo ""
echo "Removing stopped containers..."
docker container prune -f

echo ""
echo "Removing unused networks..."
docker network prune -f

echo ""
echo "Removing unused images..."
docker image prune -a -f

echo ""
echo "Removing unused volumes..."
docker volume prune -f

echo ""
echo "Removing build cache..."
docker builder prune -a -f

echo ""
echo "Final system prune pass..."
docker system prune -a -f --volumes

echo ""
echo "Disk usage after cleanup:"
docker system df

echo "========== Cleanup completed $(date) =========="
