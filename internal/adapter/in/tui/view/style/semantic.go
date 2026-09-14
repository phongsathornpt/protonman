package style

// Semantic color tokens. Components should depend on these names rather than
// raw palette values so state meaning survives future palette changes.
var (
	ColorTextPrimary   = DefaultDarkPalette.TextPrimary
	ColorTextSecondary = DefaultDarkPalette.TextSecondary
	ColorTextTertiary  = DefaultDarkPalette.TextMuted
	ColorTextMuted     = DefaultDarkPalette.TextMuted

	ColorPrimary      = DefaultDarkPalette.Brand
	ColorPrimaryHover = DefaultDarkPalette.BrandHover
	ColorPrimaryDark  = DefaultDarkPalette.BrandStrong

	ColorSuccess = DefaultDarkPalette.Success
	ColorWarning = DefaultDarkPalette.Warning
	ColorDanger  = DefaultDarkPalette.Danger
	ColorInfo    = DefaultDarkPalette.Info

	ColorBorder       = DefaultDarkPalette.Border
	ColorBorderSubtle = DefaultDarkPalette.BorderSubtle
	ColorFocus        = DefaultDarkPalette.Brand
	ColorBorderFocus  = DefaultDarkPalette.Brand

	AccentAssistant = ColorPrimary
	AccentUser      = ColorPrimaryHover
	AccentTool      = ColorTextTertiary
	AccentSystem    = ColorTextSecondary
	AccentPlan      = ColorPrimary
	AccentError     = ColorDanger
	AccentSuccess   = ColorSuccess
	AccentInfo      = ColorInfo

	CommandColor = ColorPrimaryHover
	WarningColor = ColorWarning
	PromptBorder = ColorBorder
)
