# WeChat Personal Account Bridge for Memoh
# Based on @pinixai/weixin-bot

FROM node:22-alpine

WORKDIR /app

# Install git for npm dependencies that need it
RUN apk add --no-cache git

# Install dependencies
COPY package.json tsconfig.json ./
RUN npm install

# Build @pinixai/weixin-bot SDK (installed as source)
RUN cd node_modules/@pinixai/weixin-bot \
    && npm install \
    && npx tsc

# Copy source code
COPY src ./src

# Build TypeScript
RUN npm run build

# Create data directory for WeChat credentials
RUN mkdir -p /data/.weixin-bot

# Expose control port
EXPOSE 3000

# Run the bridge
CMD ["npm", "start"]
