# syntax=docker/dockerfile:1

FROM golang:1.26-alpine AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/newapi-operator ./cmd/newapi-operator \
 && CGO_ENABLED=0 go build -trimpath \
        -ldflags "-s -w -X main.version=${VERSION}" \
        -o /out/routesync ./cmd/routesync

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/newapi-operator /usr/local/bin/newapi-operator
COPY --from=build /out/routesync /usr/local/bin/routesync

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/newapi-operator"]