package help

import (
	"fmt"

	"github.com/sjzar/chatlog/internal/ui/style"

	"github.com/rivo/tview"
)

const (
	Title     = "help"
	ShowTitle = "Help"
	Content   = `[yellow]Chatlog User Guide[white]

[green]Basic Operations:[white]
• Use [yellow]←→[white] keys to switch between main menu and help page
• Use [yellow]↑↓[white] keys to move between menu items
• Press [yellow]Enter[white] to select a menu item
• Press [yellow]Esc[white] to return to previous menu
• Press [yellow]Ctrl+C[white] to exit the program

[green]Usage Steps:[white]

[yellow]1. Download and install WeChat client[white]

[yellow]2. Migrate WeChat chat history from phone[white]
   On mobile WeChat: [yellow]Me - Settings - General - Chat History Migration & Backup - Migrate - Migrate to Computer[white].
   The purpose of this step is to transfer chat history from your phone to your computer.
   You can do this safely; it won't affect the chat history on your phone.

[yellow]3. Decrypt data[white]
   Reopen chatlog, select "Decrypt Data" menu item. The program will use the acquired key to decrypt WeChat database files.
   Decrypted files will be saved to the working directory (can be changed in settings).

[yellow]4. Start HTTP service[white]
   Select "Start HTTP Service" menu item to start HTTP and MCP services.
   After starting, visit http://localhost:5030 in your browser to view chat history.

[yellow]5. Settings[white]
   Select "Settings" menu item to configure:
   • HTTP service address - Change the HTTP service listening address
   • Working directory - Change the storage location for decrypted data

[green]HTTP API Usage:[white]
• Chat history: [yellow]GET http://localhost:5030/api/v1/chatlog?time=2023-01-01&talker=wxid_xxx[white]
• Contacts: [yellow]GET http://localhost:5030/api/v1/contact[white]
• Chat rooms: [yellow]GET http://localhost:5030/api/v1/chatroom[white]
• Sessions: [yellow]GET http://localhost:5030/api/v1/session[white]

[green]MCP Integration:[white]
Chatlog supports Model Context Protocol and can integrate with MCP-enabled AI assistants.
Through MCP, AI assistants can directly query your chat history, contacts, and chat room information.

[green]FAQ:[white]
• If key acquisition fails, ensure WeChat is running
• If decryption fails, check if the key was correctly acquired
• If HTTP service fails to start, check if the port is in use
• Data directory and working directory are saved automatically and loaded on next startup

[green]Data Security:[white]
• All data processing is done locally; nothing is uploaded to external servers
• Please store decrypted data securely to avoid privacy leaks
`
)

type Help struct {
	*tview.TextView
	title string
}

func New() *Help {
	help := &Help{
		TextView: tview.NewTextView(),
		title:    Title,
	}

	help.SetDynamicColors(true)
	help.SetRegions(true)
	help.SetWrap(true)
	help.SetTextAlign(tview.AlignLeft)
	help.SetBorder(true)
	help.SetBorderColor(style.BorderColor)
	help.SetTitle(ShowTitle)

	fmt.Fprint(help, Content)

	return help
}
