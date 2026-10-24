# Сборка: статический бинарник без cgo.
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY cmd ./cmd
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/ghreport ./cmd/ghreport

# Запуск: минимальный образ с CA-сертификатами и непривилегированным пользователем nonroot (65532).
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/ghreport /usr/local/bin/ghreport
WORKDIR /home/nonroot
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/ghreport"]
