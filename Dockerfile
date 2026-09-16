FROM golang:1.26-bookworm AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG GIT_SHA=unknown
ARG BUILD_TIME=unknown

RUN CGO_ENABLED=0 go build \
	-ldflags "-X github.com/Nico-DM/craftingo/internal/build.GitSHA=${GIT_SHA} -X github.com/Nico-DM/craftingo/internal/build.BuildTime=${BUILD_TIME}" \
	-o /craftingo .

FROM gcr.io/distroless/static-debian12

COPY --from=builder /craftingo /craftingo

EXPOSE 8899

ENTRYPOINT ["/craftingo"]
