// Package httpapi 暴露 HTTP 接口（前缀 /api），把领域服务映射为可调用 API。
package httpapi

import (
	"log"
	"net/http"
	"time"

	"task289-compverify/internal/model"
	"task289-compverify/internal/service"
)

// Server 是 HTTP 服务。
type Server struct {
	app  *service.App
	mux  *http.ServeMux
	addr string
}

// New 构造 HTTP 服务并注册全部路由。
func New(app *service.App, addr string) *Server {
	s := &Server{app: app, mux: http.NewServeMux(), addr: addr}
	s.routes()
	return s
}

// Addr 返回监听地址。
func (s *Server) Addr() string { return s.addr }

// Handler 返回根 handler（供测试与启动复用）。
func (s *Server) Handler() http.Handler { return logMiddleware(s.mux) }

// ListenAndServe 启动长驻服务。
func (s *Server) ListenAndServe() error {
	log.Printf("compverify listening on %s", s.addr)
	return http.ListenAndServe(s.addr, s.Handler())
}

func (s *Server) routes() {
	// 运行
	s.mux.HandleFunc("POST /api/runs", s.handleCreateRun)
	s.mux.HandleFunc("GET /api/runs", s.handleListRuns)
	s.mux.HandleFunc("GET /api/runs/{id}", s.handleGetRun)
	s.mux.HandleFunc("PATCH /api/runs/{id}/status", s.handleTransitionRun)
	// 事件导入
	s.mux.HandleFunc("POST /api/runs/{id}/steps", s.handleIngestStep)
	s.mux.HandleFunc("POST /api/runs/{id}/effects", s.handleIngestEffect)
	s.mux.HandleFunc("POST /api/runs/{id}/compensations", s.handleIngestCompensation)
	s.mux.HandleFunc("POST /api/runs/{id}/retries", s.handleIngestRetry)
	// 查询
	s.mux.HandleFunc("GET /api/runs/{id}/graph", s.handleGraph)
	s.mux.HandleFunc("GET /api/steps/{id}", s.handleGetStep)
	s.mux.HandleFunc("GET /api/effects/{id}", s.handleGetEffect)
	s.mux.HandleFunc("GET /api/runs/{id}/residues", s.handleListResidues)
	// 验证与裁决
	s.mux.HandleFunc("POST /api/runs/{id}/verify", s.handleVerify)
	s.mux.HandleFunc("GET /api/runs/{id}/verify", s.handleVerifyStatus)
	s.mux.HandleFunc("POST /api/compensations/{id}/confirm", s.handleConfirmComp)
	s.mux.HandleFunc("POST /api/compensations/{id}/waive", s.handleWaiveComp)
	s.mux.HandleFunc("POST /api/runs/{id}/residues/{rid}/resolve", s.handleResolveResidue)
	// 快照
	s.mux.HandleFunc("POST /api/runs/{id}/snapshots", s.handleCreateSnapshot)
	s.mux.HandleFunc("GET /api/runs/{id}/snapshots", s.handleListSnapshots)
	s.mux.HandleFunc("GET /api/snapshots/{id}", s.handleGetSnapshot)
	s.mux.HandleFunc("POST /api/snapshots/{id}/publish", s.handlePublishSnapshot)
	// 系统
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	s.mux.HandleFunc("GET /api/health", s.handleHealth)
}

// ---- handlers ----

func (s *Server) handleCreateRun(w http.ResponseWriter, r *http.Request) {
	var in struct {
		TraceID     string `json:"trace_id"`
		Description string `json:"description"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if in.TraceID == "" {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("trace_id required"))
		return
	}
	run, err := s.app.CreateRun(in.TraceID, in.Description)
	if err != nil {
		writeErr(w, http.StatusConflict, err)
		return
	}
	writeJSON(w, http.StatusCreated, run)
}

func (s *Server) handleListRuns(w http.ResponseWriter, r *http.Request) {
	limit, _ := parseInt64(r.URL.Query().Get("limit"))
	runs, err := s.app.Store.ListRuns(int(limit))
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, runs)
}

func (s *Server) handleGetRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	run, err := s.app.Store.GetRun(id)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleTransitionRun(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	var in struct {
		Status string `json:"status"`
	}
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.app.TransitionRun(id, model.RunStatus(in.Status)); err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	run, _ := s.app.Store.GetRun(id)
	writeJSON(w, http.StatusOK, run)
}

func (s *Server) handleIngestStep(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	var in ingestStepInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	step, err := s.app.Ingest.IngestStep(runID, in.toDomain())
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, step)
}

func (s *Server) handleIngestEffect(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	var in ingestEffectInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	eff, err := s.app.Ingest.IngestEffect(runID, in.toDomain())
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, eff)
}

func (s *Server) handleIngestCompensation(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	var in ingestCompInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	comp, err := s.app.Ingest.IngestCompensation(runID, in.toDomain())
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, comp)
}

func (s *Server) handleIngestRetry(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	var in ingestRetryInput
	if err := decodeJSON(r, &in); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ev, err := s.app.Ingest.IngestRetry(runID, in.toDomain())
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, ev)
}

func (s *Server) handleGraph(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	g, err := s.app.LoadGraph(runID)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, g)
}

func (s *Server) handleGetStep(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	step, err := s.app.Store.GetStep(id)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, step)
}

func (s *Server) handleGetEffect(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	eff, err := s.app.Store.GetEffect(id)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, eff)
}

func (s *Server) handleListResidues(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	residues, err := s.app.Store.ListResidues(runID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, residues)
}

func (s *Server) handleVerify(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	rep, err := s.app.VerifyRun(runID)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleVerifyStatus(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	run, err := s.app.Store.GetRun(runID)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	residues, _ := s.app.Store.ListResidues(runID)
	// 只要运行未封存，即视为验证通过
	rep := struct {
		RunID    int64           `json:"run_id"`
		OK       bool            `json:"ok"`
		Status   model.RunStatus `json:"status"`
		Residues int             `json:"residue_count"`
		Checked  time.Time       `json:"checked_at"`
	}{
		RunID:    runID,
		OK:       run.Status != model.RunSealed,
		Status:   run.Status,
		Residues: len(residues),
		Checked:  time.Now().UTC(),
	}
	writeJSON(w, http.StatusOK, rep)
}

func (s *Server) handleConfirmComp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	if err := s.app.ConfirmCompensation(id); err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	comp, _ := s.app.Store.GetCompensation(id)
	writeJSON(w, http.StatusOK, comp)
}

func (s *Server) handleWaiveComp(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	_ = decodeJSON(r, &in)
	if err := s.app.WaiveCompensation(id, in.Reason); err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	comp, _ := s.app.Store.GetCompensation(id)
	writeJSON(w, http.StatusOK, comp)
}

func (s *Server) handleResolveResidue(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	rid, ok := pathID(r, "rid")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid residue id"))
		return
	}
	var in struct {
		Note string `json:"note"`
	}
	_ = decodeJSON(r, &in)
	if err := s.app.ResolveResidue(runID, rid, in.Note); err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"resolved": true})
}

func (s *Server) handleCreateSnapshot(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	var in struct {
		Summary string `json:"summary"`
	}
	_ = decodeJSON(r, &in)
	snap, err := s.app.Snapshot.CreateDraft(runID, in.Summary)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusCreated, snap)
}

func (s *Server) handleListSnapshots(w http.ResponseWriter, r *http.Request) {
	runID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid run id"))
		return
	}
	snaps, err := s.app.Snapshot.List(runID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, snaps)
}

func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid id"))
		return
	}
	snap, err := s.app.Snapshot.Get(id)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, snap)
}

func (s *Server) handlePublishSnapshot(w http.ResponseWriter, r *http.Request) {
	snapID, ok := pathID(r, "id")
	if !ok {
		writeErr(w, http.StatusBadRequest, model.ErrBadInput("invalid snapshot id"))
		return
	}
	snap, err := s.app.Snapshot.Get(snapID)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	var in struct {
		Summary string `json:"summary"`
	}
	_ = decodeJSON(r, &in)
	published, err := s.app.PublishSnapshot(snap.RunID, snapID, in.Summary)
	if err != nil {
		writeErr(w, statusFor(err), err)
		return
	}
	writeJSON(w, http.StatusOK, published)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GlobalStats()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := s.app.Store.Ping(); err != nil {
		writeErr(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

