# syntax=docker/dockerfile:1.7

# --- build stage --------------------------------------------------------------
FROM golang:1.25-alpine AS build

WORKDIR /src

# Cache module downloads independent of source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static binary so the runtime image can stay distroless-thin.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/tracker ./cmd/server \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed    ./cmd/seed

# --- runtime stage ------------------------------------------------------------
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app

# Templates and static assets are served from the working directory at runtime.
COPY --from=build /src/web ./web
COPY --from=build /out/tracker /app/tracker
COPY --from=build /out/seed    /app/seed

EXPOSE 8080
USER nonroot:nonroot

ENTRYPOINT ["/app/tracker"]
