package footer

import (
	"fmt"

	"github.com/sjzar/chatlog/internal/ui/style"
	"github.com/sjzar/chatlog/pkg/version"

	"github.com/rivo/tview"
)

const (
	Title = "footer"
)

type Footer struct {
	*tview.Flex
	title     string
	copyRight *tview.TextView
	help      *tview.TextView
}

func New() *Footer {
	footer := &Footer{
		Flex:      tview.NewFlex(),
		title:     Title,
		copyRight: tview.NewTextView(),
		help:      tview.NewTextView(),
	}

	footer.copyRight.
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignLeft)
	footer.copyRight.
		SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)
	footer.copyRight.SetText(fmt.Sprintf("[%s::b]%s[-:-:-]", style.GetColorHex(style.PageHeaderFgColor), fmt.Sprintf(" @ Sarv's Chatlog %s", version.Version)))

	footer.help.
		SetDynamicColors(true).
		SetWrap(true).
		SetTextAlign(tview.AlignRight)
	footer.help.
		SetBackgroundColor(tview.Styles.PrimitiveBackgroundColor)

	fmt.Fprintf(footer.help,
		"[%s::b]↑/↓[%s::b]: Navigate  [%s::b]←/→[%s::b]: Switch tab  [%s::b]Enter[%s::b]: Select  [%s::b]ESC[%s::b]: Back  [%s::b]Ctrl+C[%s::b]: Exit",
		style.GetColorHex(style.MenuBgColor), style.GetColorHex(style.PageHeaderFgColor),
		style.GetColorHex(style.MenuBgColor), style.GetColorHex(style.PageHeaderFgColor),
		style.GetColorHex(style.MenuBgColor), style.GetColorHex(style.PageHeaderFgColor),
		style.GetColorHex(style.MenuBgColor), style.GetColorHex(style.PageHeaderFgColor),
		style.GetColorHex(style.MenuBgColor), style.GetColorHex(style.PageHeaderFgColor),
	)

	footer.
		AddItem(footer.copyRight, 0, 1, false).
		AddItem(footer.help, 0, 1, false)

	return footer
}

func (f *Footer) SetCopyRight(text string) {
	f.copyRight.SetText(text)
}

func (f *Footer) SetHelp(text string) {
	f.help.SetText(text)
}
