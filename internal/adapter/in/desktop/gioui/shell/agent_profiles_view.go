//go:build desktop || desktop_gio

package shell

import (
	"image"
	"strings"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"

	"github.com/phongsathornpt/protonman/internal/adapter/in/desktop/gioui/controller"
	"image/color"
)

func activeAgentDisplayName(snapshot controller.Snapshot) string {
	return "Protonman"
}

func sessionConnection(snapshot controller.Snapshot, agentID string) controller.ConnectionPhase {
	if strings.TrimSpace(agentID) == "" {
		agentID = controller.ProtonmanAgentID
	}
	return snapshot.Connection
}

func anyAgentConnected(snapshot controller.Snapshot) bool {
	return snapshot.Connection == controller.ConnectionConnected
}

func (s *Shell) layoutStatusDot(gtx layout.Context, c color.NRGBA) layout.Dimensions {
	size := gtx.Dp(8)
	gtx.Constraints.Min = image.Pt(size, size)
	gtx.Constraints.Max = image.Pt(size, size)
	paint.FillShape(gtx.Ops, c, clip.Ellipse{Max: image.Pt(size, size)}.Op(gtx.Ops))
	return layout.Dimensions{Size: image.Pt(size, size)}
}
