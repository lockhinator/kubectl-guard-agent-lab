FROM golang:1.24-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
RUN CGO_ENABLED=0 go build -o /audit-exporter .

FROM gcr.io/distroless/static:nonroot
COPY --from=build /audit-exporter /audit-exporter
EXPOSE 9099
ENTRYPOINT ["/audit-exporter"]
