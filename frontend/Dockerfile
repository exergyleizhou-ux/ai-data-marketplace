# syntax=docker/dockerfile:1

# --- deps ---
FROM node:20-alpine AS deps
WORKDIR /app
COPY package.json package-lock.json* ./
RUN npm ci || npm install

# --- build ---
FROM node:20-alpine AS build
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
ENV NEXT_TELEMETRY_DISABLED=1
# NEXT_PUBLIC_* are baked into the client bundle at build time. Pass them as
# build-args for production images; they default empty (same as a local build,
# where the app falls back to localhost / sandbox).
ARG NEXT_PUBLIC_API_BASE_URL
ARG NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY
ENV NEXT_PUBLIC_API_BASE_URL=${NEXT_PUBLIC_API_BASE_URL}
ENV NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY=${NEXT_PUBLIC_STRIPE_PUBLISHABLE_KEY}
RUN npm run build

# --- runtime (Next.js standalone output) ---
FROM node:20-alpine AS runtime
ARG OCI_REVISION=unknown
ARG OCI_VERSION=dev
ARG OCI_SOURCE=https://github.com/exergyleizhou-ux/ai-data-marketplace
LABEL org.opencontainers.image.title="Oasis frontend" \
      org.opencontainers.image.source="${OCI_SOURCE}" \
      org.opencontainers.image.revision="${OCI_REVISION}" \
      org.opencontainers.image.version="${OCI_VERSION}"
WORKDIR /app
ENV NODE_ENV=production NEXT_TELEMETRY_DISABLED=1
COPY --from=build /app/public ./public
COPY --from=build /app/.next/standalone ./
COPY --from=build /app/.next/static ./.next/static
EXPOSE 3000
CMD ["node", "server.js"]
