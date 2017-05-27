package main

import (
	"github.com/gin-gonic/gin"
	"encoding/base64"
	"encoding/csv"
	"strconv"
	"os/exec"
	"io"
	"io/ioutil"
	"os"
	"strings"
	"log"
	"time"
)

type Stream struct {
	Id	string
	Url	string
	Active	bool
	Cmd	*exec.Cmd
	LastWrite time.Time
	Playlist string
	LastChunk string
	Stderr	string
	Stdout	io.ReadCloser
}

type Source struct {
	Id	int
	Name	string
	Url	string
	UrlEncoded	string
}

type Server struct {
	Streams map[string]*Stream
	Sources []*Source
}

func (s *Server) createStream(id string) *Stream {

	iid, _ := strconv.Atoi(id)

	strm := &Stream {
		Id: id,
		LastWrite: time.Now(),
		Url: server.Sources[iid].Url,
		Active: true,
	}
	s.Streams[id] = strm

	cmd := exec.Command(
		"./ffmpeg",
		"-i", strm.Url,
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
		"-start_number", "0",
		"-hls_time", "3",
		"-hls_list_size", "10",
		"-use_localtime", "1",
		"-hls_base_url", "/data/",
		"-hls_segment_filename", "data/stream-"+id+"-%Y%m%d-%s.ts",
		"-f", "hls",
		"-method", "PUT", "http://localhost:8080/stream/"+id+"/hls.m3u8",
	)

	strm.Cmd = cmd

	log.Printf("starting stream %s", strm.Url)

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
		for time.Since(strm.LastWrite) < 30*time.Second {
			time.Sleep(time.Second*5)
		}
		log.Printf("no client on stream %s", strm.Url)
		server.stopStream(id)
		done <- true
	}()

	err = cmd.Start();
	if err != nil {
		log.Printf("%s", err)
		return nil
	}

	return strm
}

func (s *Server) getStream(id string) *Stream {
	if val, ok := s.Streams[id]; ok && val.Active {
		return val;
	} else {
		return s.createStream(id)
	}
}

func (s *Server) stopStream(id string) {
	log.Printf("stopping stream %s", id)
	if val, ok := s.Streams[id]; ok {
		val.Active = false
		val.Cmd.Process.Kill()
		//TODO: kill clients
	}
}

func (s *Stream) serveClient(c *gin.Context) {
//	c.Data(200, "applicatiom/octet-stream", []byte("\n"))
//	c.Header("Content-Type", "application/octet-stream")
	headers := `HTTP/1.1 200 OK
Content-Type: applicatiom/octet-stream

`
	_, rw, _ := c.Writer.Hijack()
	rw.Writer.Write([]byte(headers))

	for s.LastChunk == "" {
		time.Sleep(time.Second)
	}
	var err error
	for err == nil {
		toRead := s.LastChunk
		f, err := os.Open("."+toRead)
		if err != nil {
			break
		}
		_, err = io.Copy(rw.Writer, f)
		if err != nil {
			break
		}
		s.updateRead()
		for toRead == s.LastChunk {
			time.Sleep(time.Millisecond*100)
		}
	}
}

func (s *Stream) updateRead() {
	s.LastWrite = time.Now()
}

func load_sources_csv(file string, server *Server){
	f, err := os.Open(file)
	if err != nil {
		panic(err)
	}

	r := csv.NewReader(f)
	i := 0

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
				Id: i,
				Name: record[0],
				Url: record[1],
				UrlEncoded: base64.StdEncoding.EncodeToString([]byte(record[1])),
			}
			server.Sources = append(server.Sources, &src)
			i = i + 1
		}

	}
}

var server Server

func SetHeaders(c *gin.Context) {
	c.Header("Access-Control-Allow-Origin","*")
	c.Next()
}

func main() {
	router := gin.Default()
	router.Use(SetHeaders)
	router.LoadHTMLGlob("templates/*")

	server = Server{
		Streams: make(map[string]*Stream, 0),
		Sources: make([]*Source, 0),
	}

	load_sources_csv("sources.csv", &server)

	router.GET("/", func(c *gin.Context) {
		c.Redirect(307, "/static/list.html")
	})

	router.Static("/static", "./static")
	router.Static("/data", "./data")


	stream := router.Group("/stream/:id", func(c *gin.Context) {
		id := c.Param("id")

		strm := server.getStream(id)
		if strm == nil {
			c.String(500, "Unable to start stream %s", id)
			c.Abort()
		} else {
			c.Set("stream", strm)
		}
	})

	stream.GET("player.html", func(c *gin.Context) {
		id := c.Param("id")
		c.HTML(200, "player.tmpl", gin.H{
			"id": id,
		})
	})

	stream.GET("stream.ts", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)
		strm.serveClient(c)
	})

	stream.GET("hls.m3u8", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)

		strm.updateRead()
		for strm.Playlist == "" {
			time.Sleep(time.Second)
		}
		c.Data(200, "application/x-mpegURL", []byte(strm.Playlist))
	})

	stream.PUT("hls.m3u8", func(c *gin.Context) {
		strm := c.MustGet("stream").(*Stream)
		bodyR := c.Request.Body
		body, _ := ioutil.ReadAll(bodyR)
		sbody := string(body)

		strm.Playlist = sbody
		lines := strings.Split(sbody, "\n")
		strm.LastChunk = lines[len(lines)-2]
	})


	stream.GET("list.m3u", func(c *gin.Context) {
		id := c.Param("id")
		m3u := "#EXTM3U\n"
		m3u = m3u + "#EXTINF:-1,TV\n"
		m3u = m3u + "http://" + c.Request.Host + "/stream/" + id + "/stream.ts\n"
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
