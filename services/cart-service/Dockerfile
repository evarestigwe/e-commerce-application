FROM node:18-alpine AS builder
WORKDIR /app
COPY package*.json ./
RUN npm ci --only=production

FROM gcr.io/distroless/nodejs18-debian11
COPY --from=builder /app/node_modules /app/node_modules
COPY --from=builder /app/package*.json /app/
COPY . /app
WORKDIR /app
EXPOSE 8003
ENTRYPOINT ["/nodejs/bin/node", "index.js"]