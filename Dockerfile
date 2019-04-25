FROM scratch
ENV PATH=/bin

COPY grape /bin/

WORKDIR /

ENTRYPOINT ["/bin/grape"]