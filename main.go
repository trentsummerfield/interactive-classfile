package main

import (
	"encoding/hex"
	"io/ioutil"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
)

type Page struct {
	ClassFileBytes []string
	JavaSource     string
	Err            error
}

func main() {
	javaFile, javaErr := ioutil.ReadFile("static/HelloWorld.java")
	classFile, classErr := ioutil.ReadFile("static/HelloWorld.class")

	var err error
	if javaErr != nil {
		err = javaErr
	} else {
		err = classErr
	}

	hexString := hex.EncodeToString(classFile)
	var classString []string
	len := len(hexString)
	for i := 0; i < len; i += 2 {
		classString = append(classString, hexString[i:i+2])
	}

	page := Page{
		ClassFileBytes: classString,
		JavaSource:     string(javaFile),
		Err:            err,
	}

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("$PORT must be set")
	}

	r := gin.Default()
	r.LoadHTMLGlob("templates/*.tmpl.html")
	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.tmpl.html", page)
	})
	r.Run(":" + port)
}
