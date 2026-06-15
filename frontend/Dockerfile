# Build stage
FROM node:20-alpine AS builder

ARG NEXT_PUBLIC_API_URL
ENV NEXT_PUBLIC_API_URL=$NEXT_PUBLIC_API_URL

ARG NEXT_PUBLIC_GOOGLE_CLIENT_ID
ENV NEXT_PUBLIC_GOOGLE_CLIENT_ID=$NEXT_PUBLIC_GOOGLE_CLIENT_ID

WORKDIR /app

# Copy package files
COPY package*.json ./
RUN npm ci

# Copy code configuration
COPY tsconfig.json next.config.ts postcss.config.mjs tailwind.config.ts* ./
COPY src ./src
COPY public ./public

# Build the project
RUN npm run build

# Run stage
FROM node:20-alpine

WORKDIR /app

# Copy built resources and dependencies
COPY --from=builder /app/package*.json ./
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/.next ./.next
COPY --from=builder /app/public ./public
COPY --from=builder /app/src ./src

EXPOSE 3000

ENV PORT=3000
ENV NODE_ENV=production

CMD ["npm", "start"]
