# syntax=docker/dockerfile:1

FROM golang:1.22 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG REALMS_BUILD_TAGS=""
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -tags "$REALMS_BUILD_TAGS" -o /out/realms ./cmd/realms

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/realms /realms

EXPOSE 8080
ENTRYPOINT ["/realms"]
