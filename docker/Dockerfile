FROM alpine:latest
LABEL authors="xiayang1900@gmail.com"
COPY db/* /opt/db/
COPY build/ /opt/
COPY conf/* /opt/
EXPOSE 8003
WORKDIR /opt/
ENTRYPOINT ["./ip2loc"]