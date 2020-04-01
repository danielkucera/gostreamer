FROM golang:latest

WORKDIR /go/src/app
COPY . .

RUN apt-get update && apt-get install -y xz-utils

RUN curl https://johnvansickle.com/ffmpeg/builds/ffmpeg-git-amd64-static.tar.xz | tar -Jx && mv ffmpeg-git*/ffmpeg . && rm -rf ffmpeg-git*

RUN go get -d -v ./...
RUN go install -v ./...

VOLUME /go/src/app/data
EXPOSE 8080

CMD ["app"]
