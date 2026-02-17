FROM node:20-slim

RUN apt-get update && apt-get install -y --no-install-recommends git ca-certificates && rm -rf /var/lib/apt/lists/*

# Install CLIs globally
RUN npm install -g @openai/codex @anthropic-ai/claude-code @google/gemini-cli && npm cache clean --force

WORKDIR /app
COPY package.json server.js ./

EXPOSE 3100
CMD ["node", "server.js"]
