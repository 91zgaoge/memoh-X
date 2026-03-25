# WeChat Personal Account Bridge for Memoh
# Based on @pinixai/weixin-bot
# This version uses tsx to run TypeScript directly without compilation

FROM node:22-alpine

WORKDIR /app

# Install git for npm dependencies that need it
RUN apk add --no-cache git

# Copy pre-built node_modules from host
COPY node_modules ./node_modules
COPY package.json tsconfig.json ./

# Copy source code
COPY src ./src

# Create data directory for WeChat credentials
RUN mkdir -p /data/.weixin-bot

# Expose control port
EXPOSE 3000

# Run the bridge with tsx (TypeScript execution)
CMD ["node", "node_modules/.bin/tsx", "src/index.ts"]
