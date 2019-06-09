package jaal

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/appointy/idgen"
	"github.com/gorilla/websocket"
	"go.appointy.com/jaal/graphql"
)

// HTTPHandler implements the handler required for executing the graphql queries and mutations
func HTTPHandler(schema *graphql.Schema) http.Handler {
	return &httpHandler{
		handler{
			schema:   schema,
			executor: &graphql.Executor{},
		},
	}
}

// HTTPSubHandler implements the handler required for executing the graphql subscriptions
func HTTPSubHandler(schema *graphql.Schema) http.Handler {
	return &httpSubHandler{
		handler{
			schema:   schema,
			executor: &graphql.Executor{},
		},
	}
}

type handler struct {
	schema   *graphql.Schema
	executor *graphql.Executor
}

type httpHandler struct {
	handler
}

type httpSubHandler struct {
	handler
}

type httpPostBody struct {
	Query     string                 `json:"query"`
	Variables map[string]interface{} `json:"variables"`
}

type httpResponse struct {
	Data   interface{} `json:"data"`
	Errors []string    `json:"errors"`
}

type endMessage struct{}

var upgrader = websocket.Upgrader{}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	writeResponse := func(value interface{}, err error) {
		response := httpResponse{}
		if err != nil {
			response.Errors = []string{err.Error()}
		} else {
			response.Data = value
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		w.Write(responseJSON)
	}

	if r.Method != "POST" {
		writeResponse(nil, errors.New("request must be a POST"))
		return
	}

	if r.Body == nil {
		writeResponse(nil, errors.New("request must include a query"))
		return
	}

	var params httpPostBody
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeResponse(nil, err)
		return
	}

	query, err := graphql.Parse(params.Query, params.Variables)
	if err != nil {
		writeResponse(nil, err)
		return
	}

	schema := h.schema.Query
	if query.Kind == "mutation" {
		schema = h.schema.Mutation
	}

	if err := graphql.ValidateQuery(r.Context(), schema, query.SelectionSet); err != nil {
		writeResponse(nil, err)
		return
	}

	output, err := h.executor.Execute(r.Context(), schema, nil, query)
	if err != nil {
		writeResponse(nil, err)
		return
	}
	writeResponse(output, nil)

}

func (h *httpSubHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	getResponse := func(value interface{}, err error) []byte {
		response := httpResponse{}
		if err != nil {
			response.Errors = []string{err.Error()}
		} else {
			response.Data = value
		}

		responseJSON, err := json.Marshal(response)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return nil
		}
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json")
		}
		return responseJSON
	}

	fmt.Println("started")

	if r.Method != "POST" {
		res := getResponse(nil, errors.New("request must be a POST"))
		w.Write(res)
		fmt.Println("not post")
		return
	}

	if r.Body == nil {
		res := getResponse(nil, errors.New("request must include a query"))
		w.Write(res)
		return
	}

	var params httpPostBody
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		res := getResponse(nil, err)
		w.Write(res)
		return
	}

	query, err := graphql.Parse(params.Query, params.Variables)
	if err != nil {
		res := getResponse(nil, err)
		w.Write(res)
		return
	}

	subType := query.SelectionSet.Selections[0].Name

	fmt.Println("parsed")

	schema := h.schema.Subscription

	if err := graphql.ValidateQuery(r.Context(), schema, query.SelectionSet); err != nil {
		res := getResponse(nil, err)
		w.Write(res)
		return
	}

	fmt.Println("validated")

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		res := getResponse(nil, errors.New("could not establish websokcet connection"))
		w.Write(res)
		return
	}
	defer conn.Close()

	id := idgen.New("usr")

	// TODO : Add support for multiple fields in the selection set of subscription
	usrChannel := make(chan interface{})
	SubStreamManager.Lock.RLock()
	SubStreamManager.ServerTypeNotifs[subType].ServerTypeNotif <- usrChannel
	usrChannel <- id
	SubStreamManager.Lock.RUnlock()

	// External Error: Client side
	var extError = make(chan int)
	// Internal Error: Internal working
	var intError = make(chan int)

	// Check for unsubscription
	go func() {
		// TODO : Check if ReadMessage() works otherwise ReadJSON()
		_, _, err := conn.ReadMessage()
		if err != nil {
			extError <- 1
			return
		}
		extError <- 0
		return
	}()

	// Listening on usrChannel for any source event of subType
	for {
		select {
		case <-usrChannel:
			output, err := h.executor.Execute(r.Context(), schema, nil, query)
			if err != nil {
				res := getResponse(nil, err)
				conn.WriteJSON(res)
				intError <- 1
				fmt.Println(err)
			}
			SubTypeCacheManager.Lock.Lock()
			SubTypeCacheManager.CacheRead[subType]++
			SubTypeCacheManager.Lock.Unlock()

			conn.WriteJSON(getResponse(output, nil))
		case msg := <-extError:
			if msg == 0 {
				fmt.Printf("Client %v unsubscribed successfully\n", id)
				return
			} else if msg == 1 {
				fmt.Printf("Client %v disconnected\n", id)
				return
			}
		case <-intError:
			fmt.Println("Client disconnected. Internal Error")
			return
		}
	}
}
