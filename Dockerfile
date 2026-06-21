FROM golang:1.26 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/leanmcp ./cmd/leanmcp

FROM gcr.io/distroless/static-debian12
COPY --from=build /out/leanmcp /leanmcp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/leanmcp"]
