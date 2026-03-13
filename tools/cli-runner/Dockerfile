# syntax=docker/dockerfile:1.7

FROM node:20-slim

RUN --mount=type=cache,target=/var/cache/apt,sharing=locked \
    --mount=type=cache,target=/var/lib/apt,sharing=locked \
    apt-get update && apt-get install -y --no-install-recommends git ca-certificates util-linux && rm -rf /var/lib/apt/lists/*

# Install CLIs globally
ENV npm_config_cache=/root/.npm
RUN --mount=type=cache,target=/root/.npm,sharing=locked \
    npm install -g --no-fund --no-audit @openai/codex @anthropic-ai/claude-code @google/gemini-cli

WORKDIR /app
COPY package.json server.js ./

EXPOSE 3100
CMD ["node", "server.js"]
