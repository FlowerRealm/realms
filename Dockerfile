# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.22 AS build
WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
ARG REALMS_BUILD_TAGS=""
ARG REALMS_VERSION=""
ARG REALMS_COMMIT="none"
ARG REALMS_BUILD_DATE="unknown"
ARG TARGETOS
ARG TARGETARCH
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build -tags "$REALMS_BUILD_TAGS" -ldflags "-s -w -X realms/internal/version.Version=$REALMS_VERSION -X realms/internal/version.Commit=$REALMS_COMMIT -X realms/internal/version.Date=$REALMS_BUILD_DATE" -o /out/realms ./cmd/realms

FROM gcr.io/distroless/base-debian12
WORKDIR /
COPY --from=build /out/realms /realms

EXPOSE 8080
ENTRYPOINT ["/realms"]
