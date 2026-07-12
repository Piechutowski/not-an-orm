package lsp

import (
	"sync"

	"github.com/tliron/glsp"
	protocol "github.com/tliron/glsp/protocol_3_16"
	glspserver "github.com/tliron/glsp/server"
)

// Version is stamped into the initialize response.
const Version = "0.1.0"

const serverName = "edbml-ls"

// Server owns the open documents and the protocol handler.
type Server struct {
	mu   sync.Mutex
	docs map[string]*Document

	handler protocol.Handler
}

// NewServer wires up the LSP handler.
func NewServer() *Server {
	s := &Server{docs: map[string]*Document{}}
	s.handler = protocol.Handler{
		Initialize:  s.initialize,
		Initialized: func(*glsp.Context, *protocol.InitializedParams) error { return nil },
		Shutdown:    func(*glsp.Context) error { return nil },
		SetTrace:    func(*glsp.Context, *protocol.SetTraceParams) error { return nil },

		TextDocumentDidOpen:   s.didOpen,
		TextDocumentDidChange: s.didChange,
		TextDocumentDidClose:  s.didClose,

		TextDocumentCompletion:     s.completion,
		TextDocumentHover:          s.hover,
		TextDocumentDefinition:     s.definition,
		TextDocumentReferences:     s.references,
		TextDocumentRename:         s.rename,
		TextDocumentDocumentSymbol: s.documentSymbol,
	}
	return s
}

// RunStdio serves LSP over stdin/stdout until the client disconnects.
func (s *Server) RunStdio() error {
	return glspserver.NewServer(&s.handler, serverName, false).RunStdio()
}

func (s *Server) initialize(_ *glsp.Context, params *protocol.InitializeParams) (any, error) {
	capabilities := s.handler.CreateServerCapabilities()

	syncKind := protocol.TextDocumentSyncKindFull
	capabilities.TextDocumentSync = &protocol.TextDocumentSyncOptions{
		OpenClose: boolPtr(true),
		Change:    &syncKind,
	}
	capabilities.CompletionProvider = &protocol.CompletionOptions{
		TriggerCharacters: []string{".", "~", "[", ",", ":", ">", "<", "-"},
	}

	version := Version
	return protocol.InitializeResult{
		Capabilities: capabilities,
		ServerInfo: &protocol.InitializeResultServerInfo{
			Name:    serverName,
			Version: &version,
		},
	}, nil
}

// ---------------------------------------------------------------------------
// document sync

func (s *Server) didOpen(ctx *glsp.Context, params *protocol.DidOpenTextDocumentParams) error {
	doc := NewDocument(params.TextDocument.URI, params.TextDocument.Text)
	s.mu.Lock()
	s.docs[params.TextDocument.URI] = doc
	s.mu.Unlock()
	s.publishDiagnostics(ctx, doc)
	return nil
}

func (s *Server) didChange(ctx *glsp.Context, params *protocol.DidChangeTextDocumentParams) error {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil
	}
	for _, change := range params.ContentChanges {
		switch c := change.(type) {
		case protocol.TextDocumentContentChangeEventWhole:
			doc.Update(c.Text)
		case protocol.TextDocumentContentChangeEvent:
			if c.Range == nil {
				doc.Update(c.Text)
				continue
			}
			start := doc.FromLSP(c.Range.Start)
			end := doc.FromLSP(c.Range.End)
			doc.Update(doc.Text[:start] + c.Text + doc.Text[end:])
		}
	}
	s.publishDiagnostics(ctx, doc)
	return nil
}

func (s *Server) didClose(ctx *glsp.Context, params *protocol.DidCloseTextDocumentParams) error {
	s.mu.Lock()
	delete(s.docs, params.TextDocument.URI)
	s.mu.Unlock()
	// clear stale squiggles
	ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         params.TextDocument.URI,
		Diagnostics: []protocol.Diagnostic{},
	})
	return nil
}

func (s *Server) publishDiagnostics(ctx *glsp.Context, doc *Document) {
	ctx.Notify(protocol.ServerTextDocumentPublishDiagnostics, protocol.PublishDiagnosticsParams{
		URI:         doc.URI,
		Diagnostics: doc.LSPDiagnostics(),
	})
}

func (s *Server) get(uri string) *Document {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.docs[uri]
}

// ---------------------------------------------------------------------------
// language features

func (s *Server) completion(_ *glsp.Context, params *protocol.CompletionParams) (any, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	items := doc.Complete(params.Position)
	if items == nil {
		return nil, nil
	}
	return items, nil
}

func (s *Server) hover(_ *glsp.Context, params *protocol.HoverParams) (*protocol.Hover, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	return doc.Hover(params.Position), nil
}

func (s *Server) definition(_ *glsp.Context, params *protocol.DefinitionParams) (any, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	loc := doc.Definition(params.Position)
	if loc == nil {
		return nil, nil
	}
	return *loc, nil
}

func (s *Server) references(_ *glsp.Context, params *protocol.ReferenceParams) ([]protocol.Location, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	return doc.References(params.Position, params.Context.IncludeDeclaration), nil
}

func (s *Server) rename(_ *glsp.Context, params *protocol.RenameParams) (*protocol.WorkspaceEdit, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	return doc.Rename(params.Position, params.NewName)
}

func (s *Server) documentSymbol(_ *glsp.Context, params *protocol.DocumentSymbolParams) (any, error) {
	doc := s.get(params.TextDocument.URI)
	if doc == nil {
		return nil, nil
	}
	return doc.DocumentSymbols(), nil
}

func boolPtr(b bool) *bool { return &b }
