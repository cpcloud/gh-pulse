FROM gcr.io/distroless/static-debian13:nonroot@sha256:f7f8f729987ad0fdf6b05eeeae94b26e6a0f613bdf46feea7fc40f7bd72953e6

ARG TARGETPLATFORM

COPY --chmod=0555 ${TARGETPLATFORM}/gh-pulse /usr/local/bin/gh-pulse

ENTRYPOINT ["/usr/local/bin/gh-pulse"]
