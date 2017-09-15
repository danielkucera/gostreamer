FROM golang

WORKDIR /go/src/app
COPY . .

RUN apt-get update && apt-get install -y xz-utils

RUN curl https://johnvansickle.com/ffmpeg/builds/ffmpeg-git-64bit-static.tar.xz | tar -Jx && mv ffmpeg-git*/ffmpeg . && rm -rf ffmpeg-git*

RUN go-wrapper download
RUN go-wrapper install

VOLUME /go/src/app/data
EXPOSE 8080

CMD ["go-wrapper", "run"]
