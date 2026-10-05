FROM golang:1.26.3 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ENV CGO_ENABLED=0
RUN go build -trimpath -o /out/api ./cmd/api \
 && go build -trimpath -o /out/admin ./cmd/admin \
 && go build -trimpath -o /out/reconcile ./cmd/reconcile

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /out/admin /out/reconcile /app/
ENTRYPOINT ["/app/api"]
