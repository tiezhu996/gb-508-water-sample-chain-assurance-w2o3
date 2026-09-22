package handler

import (
	"strings"

	"github.com/blueship581/water-sample-chain-assurance/backend/internal/service"
	"github.com/blueship581/water-sample-chain-assurance/backend/internal/util"
	"github.com/gin-gonic/gin"
)

// ChainHandler exposes the release-chain view so the workbench can display
// link relations, live constraint checks and persisted blocking reasons.
type ChainHandler struct {
	chain service.ChainService
}

func NewChainHandler(chain service.ChainService) *ChainHandler {
	return &ChainHandler{chain: chain}
}

func (h *ChainHandler) Register(group *gin.RouterGroup) {
	group.GET("/chain/:entityType/:id", h.inspect)
}

func (h *ChainHandler) inspect(c *gin.Context) {
	id, ok := parseID(c)
	if !ok {
		return
	}
	entityType := strings.TrimSpace(c.Param("entityType"))
	view, err := h.chain.Inspect(c.Request.Context(), entityType, id)
	if err != nil {
		handleError(c, err)
		return
	}
	util.OK(c, view)
}
