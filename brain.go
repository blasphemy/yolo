package cobe

import (
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

type Brain struct {
	graph  *graph
	tok    tokenizer
	scorer scorer
}

const spaceTokenID tokenID = -1

func OpenBrain(path string) (*Brain, error) {
	graph, err := openGraph(path)
	if err != nil {
		return nil, err
	}

	version, err := graph.GetInfoString("version")
	if err != nil {
		return nil, err
	}

	if version != "2" {
		return nil, fmt.Errorf("cannot read version %s brain", version)
	}

	tokenizer, err := graph.GetInfoString("tokenizer")
	if err != nil {
		return nil, err
	}

	return &Brain{graph, getTokenizer(tokenizer), &cobeScorer{}}, nil
}

func (b *Brain) Close() {
	if b.graph != nil {
		b.graph.Close()
		b.graph = nil
	}
}

func getTokenizer(name string) tokenizer {
	switch strings.ToLower(name) {
	case "cobe":
		return newCobeTokenizer()
	case "megahal":
		return newMegaHALTokenizer()
	}

	return nil
}

func (b *Brain) Learn(text string) {
	tokens := b.tok.Split(text)

	// skip learning if too few tokens (but don't count spaces)
	if countGoodTokens(tokens) <= b.graph.getOrder() {
		return
	}

	var tokenIds []tokenID
	for _, text := range tokens {
		var tokenID tokenID
		if text == " " {
			tokenID = spaceTokenID
		} else {
			tokenID = b.graph.GetOrCreateToken(text)
		}

		tokenIds = append(tokenIds, tokenID)
	}

	var prevNode nodeID
	b.forEdges(tokenIds, func(prev, next []tokenID, hasSpace bool) {
		if prevNode == 0 {
			prevNode = b.graph.GetOrCreateNode(prev)
		}
		nextNode := b.graph.GetOrCreateNode(next)

		b.graph.addEdge(prevNode, nextNode, hasSpace)
		prevNode = nextNode
	})
}

func countGoodTokens(tokens []string) int {
	var count int
	for _, token := range tokens {
		if token != " " {
			count++
		}
	}

	return count
}

func (b *Brain) forEdges(tokenIds []tokenID, f func([]tokenID, []tokenID, bool)) {
	// Call f() on every N-gram (N = brain order) in tokenIds.
	order := b.graph.getOrder()

	chain := b.toChain(order, tokenIds)
	edges := toEdges(order, chain)

	for _, e := range edges {
		f(e.prev, e.next, e.hasSpace)
	}
}

func (b *Brain) toChain(order int, tokenIds []tokenID) []tokenID {
	var chain []tokenID
	for i := 0; i < order; i++ {
		chain = append(chain, b.graph.endTokenID)
	}

	chain = append(chain, tokenIds...)

	for i := 0; i < order; i++ {
		chain = append(chain, b.graph.endTokenID)
	}

	return chain
}

type edge struct {
	prev     []tokenID
	next     []tokenID
	hasSpace bool
}

func toEdges(order int, tokenIds []tokenID) []edge {
	var tokens []tokenID
	var spaces []int

	// Turn tokenIds (containing some SPACE_TOKEN_ID) into a list
	// of tokens and a list of positions in the tokens slice after
	// which spaces were found.

	for i := 0; i < len(tokenIds); i++ {
		tokens = append(tokens, tokenIds[i])

		if i < len(tokenIds)-1 && tokenIds[i+1] == spaceTokenID {
			spaces = append(spaces, len(tokens))
			i++
		}
	}

	var ret []edge

	prev := tokens[0:order]
	for i := 1; i < len(tokens)-order+1; i++ {
		next := tokens[i : i+order]

		var hasSpace bool
		if len(spaces) > 0 && spaces[0] == i+order-1 {
			hasSpace = true
			spaces = spaces[1:]
		}

		ret = append(ret, edge{prev, next, hasSpace})
		prev = next
	}

	return ret
}

func (b *Brain) Reply(text string) string {
	tokens := b.tok.Split(text)
	tokenIds := b.graph.filterPivots(unique(tokens))

	stemTokenIds := b.conflateStems(tokens)
	tokenIds = uniqueIds(append(tokenIds, stemTokenIds...))

	if len(tokenIds) == 0 {
		tokenIds = b.babble()
	}

	var count int

	var bestReply *reply
	var bestScore float64 = -1

	next := b.replySearch(tokenIds)

	endTime := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(endTime) {
		edges, valid := next()
		if !valid {
			// run another search
			next = b.replySearch(tokenIds)
		} else {
			reply := newReply(b.graph, edges)
			score := b.scorer.Score(reply)

			if score > bestScore {
				bestReply = reply
				bestScore = score
			}

			count++
		}
	}

	fmt.Printf("Got %d total replies\n", count)
	if bestReply == nil {
		return "no replies :  ("
	}

	return bestReply.ToString()
}

func (b *Brain) conflateStems(tokens []string) []tokenID {
	var ret []tokenID

	for _, token := range tokens {
		tokenIds := b.graph.getTokensByStem(token)
		ret = append(ret, tokenIds...)
	}

	return ret
}

func (b *Brain) babble() []tokenID {
	var tokenIds []tokenID

	for i := 0; i < 5; i++ {
		t := b.graph.getRandomToken()
		if t > 0 {
			tokenIds = append(tokenIds, tokenID(t))
		}
	}

	return tokenIds
}

// replySearch combines a forward and a reverse search over the graph
// into a series of replies.
func (b *Brain) replySearch(tokenIds []tokenID) func() ([]edgeID, bool) {
	pivotID := b.pickPivot(tokenIds)
	pivotNode := b.graph.getRandomNodeWithToken(pivotID)

	endNode := b.graph.endContextID

	revIter := b.graph.search(pivotNode, endNode, Reverse)
	fwdIter := b.graph.search(pivotNode, endNode, Forward)

	return func() ([]edgeID, bool) {
		if !revIter.Next() {
			return nil, false
		}

		if !fwdIter.Next() {
			return nil, false
		}

		return join(revIter.Result(), fwdIter.Result()), true
	}
}

func join(rev []edgeID, fwd []edgeID) []edgeID {
	edges := make([]edgeID, 0, len(rev)+len(fwd))

	// rev is a path from the pivot node to the beginning of a
	// reply: join its edges in reverse order.
	for i := len(rev) - 1; i >= 0; i-- {
		edges = append(edges, rev[i])
	}

	return append(edges, fwd...)
}

func (b *Brain) pickPivot(tokenIds []tokenID) tokenID {
	return tokenIds[rand.Intn(len(tokenIds))]
}

func unique(tokens []string) []string {
	// Reduce tokens to a unique set by sending them through a map.
	m := make(map[string]int)
	for _, token := range tokens {
		m[token]++
	}

	ret := make([]string, 0, len(m))
	for token := range m {
		ret = append(ret, token)
	}

	return ret
}

func uniqueIds(ids []tokenID) []tokenID {
	// Reduce token ids to a unique set by sending them through a map.
	m := make(map[tokenID]int)
	for _, id := range ids {
		m[id]++
	}

	ret := make([]tokenID, 0, len(m))
	for id := range m {
		ret = append(ret, id)
	}

	return ret
}

type reply struct {
	graph   *graph
	edges   []edgeID
	hasText bool
	text    string
}

func newReply(graph *graph, edges []edgeID) *reply {
	return &reply{graph, edges, false, ""}
}

func (r *reply) ToString() string {
	if !r.hasText {
		var parts []string

		for _, edge := range r.edges {
			word, hasSpace, err := r.graph.getTextByEdge(edge)
			if err != nil {
				log.Printf("ERROR: %s\n", err)
			}

			parts = append(parts, word)
			if hasSpace {
				parts = append(parts, " ")
			}
		}

		r.hasText = true
		r.text = strings.Join(parts, "")
	}

	return r.text
}