package main

import (
	"context"
	"fmt"
	"net/http"

	"github.com/appointy/idgen"
	"go.appointy.com/appointy/jaal"
	"go.appointy.com/appointy/jaal/graphql"
	"go.appointy.com/appointy/jaal/schemabuilder"
)

type channel struct {
	Id       string
	Name     string
	Email    string
	Resource resource
	Variants []variant
}

type variant struct {
	Id   string
	Name string
}

type resource struct {
	Id   string
	Name string
	Type ResourceType
}

type ResourceType int64

const (
	ONE ResourceType = iota + 1
	TWO
	THREE
	FOUR
)

type createChannelReq struct {
	Id       string
	Name     string
	Email    string
	Resource resource
	Variants []variant
}

type getChannelReq struct {
	Id string
}

// server is our graphql server.
type server struct {
	channels []channel
}

// registerQuery registers the root query type.
func (s *server) registerQuery(schema *schemabuilder.Schema) {
	obj := schema.Query()

	obj.FieldFunc("channel", func(ctx context.Context, args struct {
		In getChannelReq
	}) channel {
		for _, ch := range s.channels {
			if ch.Id == args.In.Id {
				return ch
			}
		}

		return channel{}
	})
}

// schema builds the graphql schema.
func (s *server) schema() *graphql.Schema {
	builder := schemabuilder.NewSchema()

	s.registerEnum(builder)
	s.registerMutation(builder)

	return builder.MustBuild()
}

func main() {
	// Instantiate a server, build a server, and serve the schema on port 3000.
	server := &server{
		channels: []channel{
			{
				Name:  "Table",
				Id:    idgen.New("ch"),
				Email: "table@appointy.com",
				Resource: resource{
					Id:   idgen.New("res"),
					Name: "channel",
				},
			},
		},
	}

	fmt.Println(server)

	schema := server.schema()
	http.Handle("/graphql", jaal.HTTPHandler(schema))
	fmt.Println("Running")

	http.ListenAndServe(":3000", nil)
}
