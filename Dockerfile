# Stage 1: Build
FROM golang:1.24-alpine AS builder

# Install git
RUN apk add --no-cache git

WORKDIR /app

# Copy dependency
# (Pakai tanda * supaya kalau go.sum belum ada, dia gak error)
COPY go.mod go.sum* ./
RUN go mod download

# Copy source code
COPY . .

# Build aplikasi
# TAMBAHAN PENTING: CGO_ENABLED=0
# Ini memaksa Go bikin binary yang bisa jalan di Alpine tanpa library C tambahan
RUN CGO_ENABLED=0 GOOS=linux go build -o binary ./cmd/api/main.go

# Stage 2: Run
FROM alpine:latest

WORKDIR /app

# Install sertifikat SSL (Supabase butuh ini)
RUN apk add --no-cache ca-certificates

# Copy binary dari stage 1
COPY --from=builder /app/binary .

EXPOSE 8080

CMD ["./binary"]