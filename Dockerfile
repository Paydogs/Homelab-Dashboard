# --- build stage -------------------------------------------------------------
FROM golang:1.27-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN apk add --no-cache tzdata \
 && CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags "-s -w" \
      -o /out/dashboard ./cmd/dashboard \
 && mkdir -p /out/config /out/defaults \
 && cp -r defaults/. /out/defaults/ \
 && cp config.example.yml /out/defaults/config.yml

# --- final stage: static binary on scratch -----------------------------------
FROM scratch AS final

# CA certificates for HTTPS healthchecks and icon downloads; time zone data so
# healthcheck timestamps render in the configured local time.
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo

# /data is the bind-mount point — the dashboard's whole data folder
# (config.yml, resources/icons cache, resources/styles + overwrite files)
# lives here on the host, never inside the image. /defaults carries the
# first-boot sample config + starter styles, copied out by the app when
# /data has no config.yml yet.
COPY --from=build --chown=1000:1000 /out/config /data
COPY --from=build /out/defaults /defaults
COPY --from=build /out/dashboard /dashboard

ENV TZ=Etc/UTC
USER 1000:1000
EXPOSE 8080
ENTRYPOINT ["/dashboard", "-data", "/data", "-listen", ":8080", "-defaults", "/defaults"]