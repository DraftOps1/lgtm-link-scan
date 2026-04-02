FROM golang:1.22.5 AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/demoapp ./examples/docker-compose/demoapp

FROM gcr.io/distroless/base-debian12:nonroot

COPY --from=build /out/demoapp /demoapp

ENTRYPOINT ["/demoapp"]
