FROM golang:alpine

WORKDIR /go/src/app
COPY . .

RUN apk update && apk upgrade && \
    apk add --no-cache bash git gcc libc-dev curl xz

RUN curl https://johnvansickle.com/ffmpeg/builds/ffmpeg-git-64bit-static.tar.xz | tar -Jx && mv ffmpeg-git*/ffmpeg . && rm -rf ffmpeg-git*

RUN go-wrapper download
RUN go-wrapper install

EXPOSE 8080

CMD ["go-wrapper", "run"]
