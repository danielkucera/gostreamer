package main

import (
	"github.com/gin-gonic/gin"
	"encoding/base64"
	"encoding/csv"
	"os/exec"
	"bufio"
	"io"
	"os"
	"log"
	"time"
)

type Stream struct {
	Url	string
	Active	bool
	Cmd	*exec.Cmd
	LastWrite time.Time
	Stderr	string
	Stdout	io.ReadCloser
	Clients map[*bufio.ReadWriter]bool `json:"-"`
//	Clients []*gin.Context `json:"-"`
}

type Source struct {
	Name	string
	Url	string
	UrlEncoded	string
}

type Server struct {
	Streams map[string]*Stream
	Sources []*Source
}

func (s *Server) createStream(Url string) *Stream {
	cmd := exec.Command(
		"./ffmpeg",
		"-i", Url,
		"-map", "0",
		"-copy_unknown",
		"-sn", //subtitle none
		"-dn", //data none
		"-deinterlace",
		"-c:v", "h264",
		"-preset", "fast",
		"-b:v", "1024k",
		"-c:a", "aac",
		"-b:a", "192k",
		"-f", "mpegts", "-",
	)

	strm := &Stream {
		Url: Url,
		Active: true,
		Cmd: cmd,
		Clients: make(map[*bufio.ReadWriter]bool, 0),
	}
	s.Streams[Url] = strm

	log.Printf("starting stream %s", strm.Url)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		log.Printf("%s", err)
		return nil
	}
	strm.Stdout = stdout

	stderr, err := cmd.StderrPipe()
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	done := make(chan bool)

	go func() {
		buf := make([]byte, 1024)
		var err error
		var lng int
		for err == nil {
			lng, err = stderr.Read(buf)
			strm.Stderr = strm.Stderr + string(buf)[0:lng]
		}
		done <- true
	}()

	go func() {
		buf := make([]byte, 1024)
		var rerr error
		var lng int
		for rerr == nil {
			lng, rerr = stdout.Read(buf)
//			log.Printf("stdout read")
			for client, _ := range strm.Clients {
				strm.LastWrite = time.Now()
//				log.Printf("client write")
				_, err := client.Writer.Write(buf[0:lng])
				if err != nil {
					log.Printf("error writing to client: %s", err)
					delete(strm.Clients, client)
				}
			}
			if time.Since(strm.LastWrite) > 30*time.Second {
				log.Printf("no client on stream %s", strm.Url)
				break
			}
		}
		log.Printf("stdout read error %s", rerr)
		server.stopStream(strm.Url)
		done <- true
	}()

	err = cmd.Start();
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	return strm
}

func (s *Server) getStream(Url string) *Stream {
	if val, ok := s.Streams[Url]; ok && val.Active {
		return val;
	} else {
		return s.createStream(Url)
	}
}

func (s *Server) stopStream(Url string) {
	log.Printf("stopping stream %s", Url)
	if val, ok := s.Streams[Url]; ok {
		val.Active = false
		val.Cmd.Process.Kill()
		//TODO: kill clients
	}
}

func (s *Stream) addClient(c *gin.Context) {
//	c.Data(200, "applicatiom/octet-stream", []byte("\n"))
//	c.Header("Content-Type", "application/octet-stream")
	headers := `HTTP/1.1 200 OK
Content-Type: applicatiom/octet-stream

`
	_, rw, _ := c.Writer.Hijack()
	rw.Writer.Write([]byte(headers))
	s.Clients[rw] = true
}

func load_sources_csv(file string, server *Server){
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}

	r := csv.NewReader(f)

	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Fatal(err)
		}

		if len(record) > 1 {
			src := Source {
				Name: record[0],
				Url: record[1],
				UrlEncoded: base64.StdEncoding.EncodeToString([]byte(record[1])),
			}
			server.Sources = append(server.Sources, &src)
		}

	}
}

var server Server

func main() {
	router := gin.Default()

	server = Server{
		Streams: make(map[string]*Stream, 0),
		Sources: make([]*Source, 0),
	}

	load_sources_csv("sources.csv", &server)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(307, "/static/list.html")
	})

	router.Static("/static", "./static")

	router.GET("/stream/:id", func(c *gin.Context) {
		id := c.Param("id")
		Url, err := base64.StdEncoding.DecodeString(id)
		if err != nil {
			c.String(500, "Unable to decode stream id %s", id)
			return
		}

		strm := server.getStream(string(Url))
		if strm == nil {
			c.String(200, "Unable to start stream %s", Url)
		} else {
			strm.addClient(c)
		}
	})

	router.GET("/list/:id", func(c *gin.Context) {
		id := c.Param("id")
		m3u := "#EXTM3U\n"
		m3u = m3u + "#EXTINF:-1,TV\n"
		m3u = m3u + "http://" + c.Request.Host + "/stream/" + id + "\n"
		c.Data(200, "audio/mpegurl", []byte(m3u))
	})

	router.GET("/status", func(c *gin.Context) {
		c.JSON(200, server)
	})

	router.GET("/sources", func(c *gin.Context) {
		c.JSON(200, server.Sources)
	})

	router.Run(":8080")
}
