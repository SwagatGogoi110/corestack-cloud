package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/aws/smithy-go/private/protocol"
)

var version = "dev"

func main()  {
	reg := protocol.New
}