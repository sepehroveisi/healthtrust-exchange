FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /hospital-node ./cmd/hospital-node

FROM alpine:3.21
RUN adduser -D -H healthtrust
USER healthtrust
COPY --from=build /hospital-node /hospital-node
ENTRYPOINT ["/hospital-node"]
