FROM node:25.8-alpine3.23 AS frontend-builder

WORKDIR /app/ui

COPY ui/package.json ui/pnpm-lock.yaml ./

RUN npm install -g pnpm && \
    pnpm install --frozen-lockfile

COPY ui/ ./
RUN pnpm run build

FROM golang:1.26.1-alpine3.23 AS backend-builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=frontend-builder /app/static ./static

RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o kite .

FROM gcr.io/distroless/static:nonroot

WORKDIR /app

COPY --from=backend-builder --chown=nonroot:nonroot /app/kite .

# Default sqlite path in a dir writable by the nonroot user (UID 65532).
# /home/nonroot is pre-created with correct ownership in distroless:nonroot.
# Override DB_DSN at runtime for external databases.
ENV DB_DSN=/home/nonroot/kite.db

EXPOSE 8080

ENTRYPOINT ["./kite"]