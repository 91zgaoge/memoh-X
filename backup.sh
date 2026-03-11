#!/bin/bash
# Memoh-v2 项目备份脚本
# 用法: ./backup.sh [备份目录]

set -e

# 配置
PROJECT_DIR="/data2/memoh-v2"
BACKUP_BASE="${1:-/data2/backups}"
TIMESTAMP=$(date +%Y%m%d_%H%M%S)
BACKUP_DIR="${BACKUP_BASE}/memoh_backup_${TIMESTAMP}"
PROJECT_NAME="memoh-v2"

echo "========================================"
echo "Memoh-v2 项目备份"
echo "========================================"
echo "备份时间: $(date)"
echo "项目目录: ${PROJECT_DIR}"
echo "备份目录: ${BACKUP_DIR}"
echo ""

# 创建备份目录
mkdir -p "${BACKUP_DIR}"

# 1. 备份代码仓库
echo "[1/5] 备份代码仓库..."
if [ -d "${PROJECT_DIR}/.git" ]; then
    cd "${PROJECT_DIR}"
    git bundle create "${BACKUP_DIR}/${PROJECT_NAME}_git.bundle" --all 2>/dev/null || echo "Git bundle 失败，使用 tar 备份"
    tar czf "${BACKUP_DIR}/${PROJECT_NAME}_code.tar.gz" \
        --exclude='node_modules' \
        --exclude='.git/objects' \
        --exclude='dist' \
        --exclude='build' \
        -C "$(dirname ${PROJECT_DIR})" \
        "$(basename ${PROJECT_DIR})"
else
    tar czf "${BACKUP_DIR}/${PROJECT_NAME}_code.tar.gz" \
        --exclude='node_modules' \
        --exclude='dist' \
        --exclude='build' \
        -C "$(dirname ${PROJECT_DIR})" \
        "$(basename ${PROJECT_DIR})"
fi

# 2. 备份 Docker 数据卷
echo "[2/5] 备份 Docker 数据卷..."
mkdir -p "${BACKUP_DIR}/volumes"

# 备份 postgres 数据
echo "  - 备份 PostgreSQL 数据..."
docker run --rm --volumes-from memoh-postgres \
    -v "${BACKUP_DIR}/volumes":/backup \
    alpine tar czf /backup/postgres_data.tar.gz -C /var/lib/postgresql/data . 2>/dev/null || echo "  跳过: PostgreSQL 容器未运行"

# 备份 qdrant 数据
echo "  - 备份 Qdrant 数据..."
docker run --rm --volumes-from memoh-qdrant \
    -v "${BACKUP_DIR}/volumes":/backup \
    alpine tar czf /backup/qdrant_data.tar.gz -C /qdrant/storage . 2>/dev/null || echo "  跳过: Qdrant 容器未运行"

# 3. 备份 Docker 配置
echo "[3/5] 备份 Docker Compose 配置..."
cp "${PROJECT_DIR}/docker-compose.yml" "${BACKUP_DIR}/" 2>/dev/null || echo "跳过: docker-compose.yml 不存在"
cp "${PROJECT_DIR}/.env" "${BACKUP_DIR}/" 2>/dev/null || echo "跳过: .env 不存在"

# 4. 备份容器镜像列表
echo "[4/5] 备份容器镜像信息..."
docker compose -f "${PROJECT_DIR}/docker-compose.yml" config > "${BACKUP_DIR}/docker-compose-resolved.yml" 2>/dev/null || echo "跳过: 无法解析 compose 配置"

# 保存镜像列表
docker ps --filter "name=memoh-" --format "table {{.Names}}\t{{.Image}}\t{{.Status}}" > "${BACKUP_DIR}/running_containers.txt" 2>/dev/null || true

# 5. 生成备份信息文件
echo "[5/5] 生成备份信息..."
cat > "${BACKUP_DIR}/BACKUP_INFO.txt" << EOL
========================================
Memoh-v2 项目备份
========================================

备份时间: $(date)
主机名: $(hostname)
用户: $(whoami)

备份内容:
1. ${PROJECT_NAME}_code.tar.gz - 项目源代码
2. volumes/postgres_data.tar.gz - PostgreSQL 数据
3. volumes/qdrant_data.tar.gz - Qdrant 向量数据
4. docker-compose.yml - Docker Compose 配置
5. docker-compose-resolved.yml - 解析后的配置
6. running_containers.txt - 运行中的容器列表

恢复说明:
1. 解压代码: tar xzf ${PROJECT_NAME}_code.tar.gz
2. 恢复数据库: docker run --rm --volumes-from memoh-postgres -v \$(pwd)/volumes:/backup alpine tar xzf /backup/postgres_data.tar.gz -C /var/lib/postgresql/data
3. 恢复 Qdrant: docker run --rm --volumes-from memoh-qdrant -v \$(pwd)/volumes:/backup alpine tar xzf /backup/qdrant_data.tar.gz -C /qdrant/storage
4. 启动服务: docker compose up -d

项目位置: ${PROJECT_DIR}
备份位置: ${BACKUP_DIR}

========================================
EOL

# 生成文件列表
echo "" >> "${BACKUP_DIR}/BACKUP_INFO.txt"
echo "备份文件清单:" >> "${BACKUP_DIR}/BACKUP_INFO.txt"
echo "========================================" >> "${BACKUP_DIR}/BACKUP_INFO.txt"
ls -lh "${BACKUP_DIR}" >> "${BACKUP_DIR}/BACKUP_INFO.txt"
ls -lh "${BACKUP_DIR}/volumes" >> "${BACKUP_DIR}/BACKUP_INFO.txt" 2>/dev/null || true

echo ""
echo "========================================"
echo "备份完成!"
echo "========================================"
echo "备份位置: ${BACKUP_DIR}"
echo ""
echo "备份文件列表:"
ls -lh "${BACKUP_DIR}"
echo ""

# 创建最新备份链接
ln -sf "${BACKUP_DIR}" "${BACKUP_BASE}/memoh_backup_latest"
echo "最新备份链接: ${BACKUP_BASE}/memoh_backup_latest"
