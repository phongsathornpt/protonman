package runtime

const (
	wideLayoutMinWidth      = 96
	wideLayoutMinHeight     = 20
	compactLayoutMinWidth   = 56
	compactLayoutMinHeight  = 14
	headerVisibleMinHeight  = 10
	overlayHeaderMinHeight  = 18
	defaultHorizontalInset  = 1
	minimumViewportHeight   = 1
)

type layoutMode uint8

const (
	layoutModeMinimal layoutMode = iota
	layoutModeCompact
	layoutModeWide
)

type layoutProfile struct {
	mode            layoutMode
	showHeader      bool
	horizontalInset int
}

func resolveLayoutProfile(width, height int, hasBottomView bool) layoutProfile {
	profile := layoutProfile{
		mode:            layoutModeWide,
		showHeader:      true,
		horizontalInset: defaultHorizontalInset,
	}

	switch {
	case width < compactLayoutMinWidth || height < compactLayoutMinHeight:
		profile.mode = layoutModeMinimal
	case width < wideLayoutMinWidth || height < wideLayoutMinHeight:
		profile.mode = layoutModeCompact
	}

	if height < headerVisibleMinHeight || (hasBottomView && height < overlayHeaderMinHeight) {
		profile.showHeader = false
	}
	return profile
}

func (p layoutProfile) contentWidth(terminalWidth int) int {
	width := terminalWidth - (p.horizontalInset * 2)
	if width < 1 {
		return 1
	}
	return width
}

func (p layoutProfile) compactHeader() bool {
	return p.mode != layoutModeWide
}

func (p layoutProfile) minimalHeader() bool {
	return p.mode == layoutModeMinimal
}

type frameGeometry struct {
	terminalWidth  int
	terminalHeight int
	chromeHeight   int
	viewportHeight int
}

func resolveFrameGeometry(width, height, chromeHeight int) frameGeometry {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if chromeHeight < 0 {
		chromeHeight = 0
	}

	viewportHeight := height - chromeHeight
	if viewportHeight < minimumViewportHeight {
		viewportHeight = minimumViewportHeight
	}

	return frameGeometry{
		terminalWidth:  width,
		terminalHeight: height,
		chromeHeight:   chromeHeight,
		viewportHeight: viewportHeight,
	}
}
