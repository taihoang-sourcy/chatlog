package chatlog

import (
	"fmt"
	"path/filepath"
	"runtime"
	"time"

	"github.com/sjzar/chatlog/internal/chatlog/ctx"
	"github.com/sjzar/chatlog/internal/model"
	"github.com/sjzar/chatlog/internal/ui/footer"
	"github.com/sjzar/chatlog/internal/ui/form"
	"github.com/sjzar/chatlog/internal/ui/help"
	"github.com/sjzar/chatlog/internal/ui/infobar"
	"github.com/sjzar/chatlog/internal/ui/menu"
	"github.com/sjzar/chatlog/internal/wechat"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/tview"
)

const (
	RefreshInterval = 1000 * time.Millisecond
)

type App struct {
	*tview.Application

	ctx         *ctx.Context
	m           *Manager
	stopRefresh chan struct{}

	// page
	mainPages *tview.Pages
	infoBar   *infobar.InfoBar
	tabPages  *tview.Pages
	footer    *footer.Footer

	// tab
	menu      *menu.Menu
	help      *help.Help
	activeTab int
	tabCount  int
}

func NewApp(ctx *ctx.Context, m *Manager) *App {
	app := &App{
		ctx:         ctx,
		m:           m,
		Application: tview.NewApplication(),
		mainPages:   tview.NewPages(),
		infoBar:     infobar.New(),
		tabPages:    tview.NewPages(),
		footer:      footer.New(),
		menu:        menu.New("Main Menu"),
		help:        help.New(),
	}

	app.initMenu()

	app.updateMenuItemsState()

	return app
}

func (a *App) Run() error {

	flex := tview.NewFlex().
		SetDirection(tview.FlexRow).
		AddItem(a.infoBar, infobar.InfoBarViewHeight, 0, false).
		AddItem(a.tabPages, 0, 1, true).
		AddItem(a.footer, 1, 1, false)

	a.mainPages.AddPage("main", flex, true, true)

	a.tabPages.
		AddPage("0", a.menu, true, true).
		AddPage("1", a.help, true, false)
	a.tabCount = 2

	a.SetInputCapture(a.inputCapture)

	go a.refresh()

	if err := a.SetRoot(a.mainPages, true).EnableMouse(false).Run(); err != nil {
		return err
	}

	return nil
}

func (a *App) Stop() {
	// 添加一个通道用于停止刷新 goroutine
	if a.stopRefresh != nil {
		close(a.stopRefresh)
	}
	a.Application.Stop()
}

func (a *App) updateMenuItemsState() {
	// 查找并更新自动解密菜单项
	for _, item := range a.menu.GetItems() {
		// 更新自动解密菜单项
			if item.Index == 5 {
			if a.ctx.AutoDecrypt {
				item.Name = "Stop Auto Decrypt"
				item.Description = "Stop monitoring data directory, no longer auto-decrypt new data"
			} else {
				item.Name = "Enable Auto Decrypt"
				item.Description = "Monitor data directory and auto-decrypt new data"
			}
		}

		// Update HTTP service menu item
		if item.Index == 4 {
			if a.ctx.HTTPEnabled {
				item.Name = "Stop HTTP Service"
				item.Description = "Stop local HTTP & MCP server"
			} else {
				item.Name = "Start HTTP Service"
				item.Description = "Start local HTTP & MCP server"
			}
		}
	}
}

func (a *App) switchTab(step int) {
	index := (a.activeTab + step) % a.tabCount
	if index < 0 {
		index = a.tabCount - 1
	}
	a.activeTab = index
	a.tabPages.SwitchToPage(fmt.Sprint(a.activeTab))
}

func (a *App) refresh() {
	tick := time.NewTicker(RefreshInterval)
	defer tick.Stop()

	for {
		select {
		case <-a.stopRefresh:
			return
		case <-tick.C:
			if a.ctx.AutoDecrypt || a.ctx.HTTPEnabled {
				a.m.RefreshSession()
			}
			a.infoBar.UpdateAccount(a.ctx.Account)
			a.infoBar.UpdateBasicInfo(a.ctx.PID, a.ctx.FullVersion, a.ctx.ExePath)
			a.infoBar.UpdateStatus(a.ctx.Status)
			a.infoBar.UpdateDataKey(a.ctx.DataKey)
			a.infoBar.UpdateImageKey(a.ctx.ImgKey)
			a.infoBar.UpdatePlatform(a.ctx.Platform)
			a.infoBar.UpdateDataUsageDir(a.ctx.DataUsage, a.ctx.DataDir)
			a.infoBar.UpdateWorkUsageDir(a.ctx.WorkUsage, a.ctx.WorkDir)
			if a.ctx.LastSession.Unix() > 1000000000 {
				a.infoBar.UpdateSession(a.ctx.LastSession.Format("2006-01-02 15:04:05"))
			}
			if a.ctx.HTTPEnabled {
				a.infoBar.UpdateHTTPServer(fmt.Sprintf("[green][Started][white] [%s]", a.ctx.HTTPAddr))
			} else {
				a.infoBar.UpdateHTTPServer("[Not started]")
			}
			if a.ctx.AutoDecrypt {
				a.infoBar.UpdateAutoDecrypt("[green][Enabled][white]")
			} else {
				a.infoBar.UpdateAutoDecrypt("[Not enabled]")
			}

			a.Draw()
		}
	}
}

func (a *App) inputCapture(event *tcell.EventKey) *tcell.EventKey {

	// 如果当前页面不是主页面，ESC 键返回主页面
	if a.mainPages.HasPage("submenu") && event.Key() == tcell.KeyEscape {
		a.mainPages.RemovePage("submenu")
		a.mainPages.SwitchToPage("main")
		return nil
	}

	if a.tabPages.HasFocus() {
		switch event.Key() {
		case tcell.KeyLeft:
			a.switchTab(-1)
			return nil
		case tcell.KeyRight:
			a.switchTab(1)
			return nil
		}
	}

	switch event.Key() {
	case tcell.KeyCtrlC:
		a.Stop()
	}

	return event
}

func (a *App) initMenu() {
	getDataKey := &menu.Item{
		Index:       2,
		Name:        "Get Key",
		Description: "Extract data key & image key from process",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal()
			if runtime.GOOS == "darwin" {
				modal.SetText("Getting key...\nThis may take about 20 seconds. WeChat may freeze during this time. Please wait.")
			} else {
				modal.SetText("Getting key...")
			}
			a.mainPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)

			go func() {
				err := a.m.GetDataKey()

				// 在主线程中更新UI
				a.QueueUpdateDraw(func() {
					if err != nil {
						modal.SetText("Failed to get key: " + err.Error())
					} else {
						modal.SetText("Key acquired successfully")
					}

					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						a.mainPages.RemovePage("modal")
					})
					a.SetFocus(modal)
				})
			}()
		},
	}

	decryptData := &menu.Item{
		Index:       3,
		Name:        "Decrypt Data",
		Description: "Decrypt data files",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal().
				SetText("Decrypting...")

			a.mainPages.AddPage("modal", modal, true, true)
			a.SetFocus(modal)

			// 在后台执行解密操作
			go func() {
				// 执行解密
				err := a.m.DecryptDBFiles()

				// 在主线程中更新UI
				a.QueueUpdateDraw(func() {
					if err != nil {
						modal.SetText("Decryption failed: " + err.Error())
					} else {
						modal.SetText("Decryption completed successfully")
					}

					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
						a.mainPages.RemovePage("modal")
					})
					a.SetFocus(modal)
				})
			}()
		},
	}

	httpServer := &menu.Item{
		Index:       4,
		Name:        "Start HTTP Service",
		Description: "Start local HTTP & MCP server",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal()

			// 根据当前服务状态执行不同操作
			if !a.ctx.HTTPEnabled {
				modal.SetText("Starting HTTP service...")
				a.mainPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				// 在后台启动服务
				go func() {
					err := a.m.StartService()

					// 在主线程中更新UI
					a.QueueUpdateDraw(func() {
						if err != nil {
							modal.SetText("Failed to start HTTP service: " + err.Error())
						} else {
							modal.SetText("HTTP service started")
						}

						a.updateMenuItemsState()

						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.mainPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			} else {
				modal.SetText("Stopping HTTP service...")
				a.mainPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				// 在后台停止服务
				go func() {
					err := a.m.StopService()

					// 在主线程中更新UI
					a.QueueUpdateDraw(func() {
						if err != nil {
							modal.SetText("Failed to stop HTTP service: " + err.Error())
						} else {
							modal.SetText("HTTP service stopped")
						}

						a.updateMenuItemsState()

						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.mainPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			}
		},
	}

	autoDecrypt := &menu.Item{
		Index:       5,
		Name:        "Enable Auto Decrypt",
		Description: "Auto-decrypt new data files",
		Selected: func(i *menu.Item) {
			modal := tview.NewModal()

			if !a.ctx.AutoDecrypt {
				modal.SetText("Enabling auto decrypt...")
				a.mainPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				// 在后台开启自动解密
				go func() {
					err := a.m.StartAutoDecrypt()

					// 在主线程中更新UI
					a.QueueUpdateDraw(func() {
						if err != nil {
							modal.SetText("Failed to enable auto decrypt: " + err.Error())
						} else {
							if a.ctx.Version == 3 {
								modal.SetText("Auto decrypt enabled\nNote: v3.x data files may update slowly; use v4.0 for lower latency")
							} else {
								modal.SetText("Auto decrypt enabled")
							}
						}

						a.updateMenuItemsState()

						// 添加确认按钮
						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.mainPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			} else {
				modal.SetText("Stopping auto decrypt...")
				a.mainPages.AddPage("modal", modal, true, true)
				a.SetFocus(modal)

				// 在后台停止自动解密
				go func() {
					err := a.m.StopAutoDecrypt()

					// 在主线程中更新UI
					a.QueueUpdateDraw(func() {
						if err != nil {
							modal.SetText("Failed to stop auto decrypt: " + err.Error())
						} else {
							modal.SetText("Auto decrypt stopped")
						}

						a.updateMenuItemsState()

						modal.AddButtons([]string{"OK"})
						modal.SetDoneFunc(func(buttonIndex int, buttonLabel string) {
							a.mainPages.RemovePage("modal")
						})
						a.SetFocus(modal)
					})
				}()
			}
		},
	}

	setting := &menu.Item{
		Index:       6,
		Name:        "Settings",
		Description: "Configure application options",
		Selected:    a.settingSelected,
	}

	selectAccount := &menu.Item{
		Index:       7,
		Name:        "Switch Account",
		Description: "Switch to another account (process or history)",
		Selected:    a.selectAccountSelected,
	}

	supplierMapping := &menu.Item{
		Index:       8,
		Name:        "Supplier Mapping",
		Description: "Map conversations to supplier IDs for sync",
		Selected:    a.supplierMappingSelected,
	}

	syncPostgres := &menu.Item{
		Index:       9,
		Name:        "Sync to PostgreSQL",
		Description: "Sync mapped conversations to PostgreSQL",
		Selected:    a.syncSelected,
	}

	a.menu.AddItem(getDataKey)
	a.menu.AddItem(decryptData)
	a.menu.AddItem(httpServer)
	a.menu.AddItem(autoDecrypt)
	a.menu.AddItem(setting)
	a.menu.AddItem(selectAccount)
	a.menu.AddItem(supplierMapping)
	a.menu.AddItem(syncPostgres)

	a.menu.AddItem(&menu.Item{
		Index:       10,
		Name:        "Exit",
		Description: "Exit the program",
		Selected: func(i *menu.Item) {
			a.Stop()
		},
	})
}

// settingItem 表示一个设置项
type settingItem struct {
	name        string
	description string
	action      func()
}

func (a *App) settingSelected(i *menu.Item) {

	settings := []settingItem{
		{
			name:        "HTTP Service Address",
			description: "Configure HTTP service listening address",
			action:      a.settingHTTPPort,
		},
		{
			name:        "Working Directory",
			description: "Configure storage directory for decrypted data",
			action:      a.settingWorkDir,
		},
		{
			name:        "Data Key",
			description: "Configure data decryption key",
			action:      a.settingDataKey,
		},
		{
			name:        "Image Key",
			description: "Configure image decryption key",
			action:      a.settingImgKey,
		},
		{
			name:        "Data Directory",
			description: "Configure WeChat data file directory",
			action:      a.settingDataDir,
		},
	}

	subMenu := menu.NewSubMenu("Settings")
	for idx, setting := range settings {
		item := &menu.Item{
			Index:       idx + 1,
			Name:        setting.name,
			Description: setting.description,
			Selected: func(action func()) func(*menu.Item) {
				return func(*menu.Item) {
					action()
				}
			}(setting.action),
		}
		subMenu.AddItem(item)
	}

	a.mainPages.AddPage("submenu", subMenu, true, true)
	a.SetFocus(subMenu)
}

// settingHTTPPort 设置 HTTP 端口
func (a *App) settingHTTPPort() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("HTTP Address")

	tempHTTPAddr := a.ctx.HTTPAddr

	formView.AddInputField("Address", tempHTTPAddr, 0, nil, func(text string) {
		tempHTTPAddr = text
	})

	formView.AddButton("Save", func() {
		a.m.SetHTTPAddr(tempHTTPAddr)
		a.mainPages.RemovePage("submenu2")
		a.showInfo("HTTP address set to " + a.ctx.HTTPAddr)
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingWorkDir 设置工作目录
func (a *App) settingWorkDir() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("Working Directory")

	tempWorkDir := a.ctx.WorkDir

	formView.AddInputField("Working Directory", tempWorkDir, 0, nil, func(text string) {
		tempWorkDir = text
	})

	formView.AddButton("Save", func() {
		a.ctx.SetWorkDir(tempWorkDir)
		a.mainPages.RemovePage("submenu2")
		a.showInfo("Working directory set to " + a.ctx.WorkDir)
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingDataKey 设置数据密钥
func (a *App) settingDataKey() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("Data Key")

	tempDataKey := a.ctx.DataKey

	formView.AddInputField("Data Key", tempDataKey, 0, nil, func(text string) {
		tempDataKey = text
	})

	formView.AddButton("Save", func() {
		a.ctx.DataKey = tempDataKey
		a.mainPages.RemovePage("submenu2")
		a.showInfo("Data key has been set")
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingImgKey 设置图片密钥 (ImgKey)
func (a *App) settingImgKey() {
	formView := form.NewForm("Image Key")

	tempImgKey := a.ctx.ImgKey

	formView.AddInputField("Image Key", tempImgKey, 0, nil, func(text string) {
		tempImgKey = text
	})

	formView.AddButton("Save", func() {
		a.ctx.SetImgKey(tempImgKey)
		a.mainPages.RemovePage("submenu2")
		a.showInfo("Image key has been set")
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// settingDataDir 设置数据目录
func (a *App) settingDataDir() {
	// 使用我们的自定义表单组件
	formView := form.NewForm("Data Directory")

	tempDataDir := a.ctx.DataDir

	formView.AddInputField("Data Directory", tempDataDir, 0, nil, func(text string) {
		tempDataDir = text
	})

	formView.AddButton("Save", func() {
		a.ctx.DataDir = tempDataDir
		a.mainPages.RemovePage("submenu2")
		a.showInfo("Data directory set to " + a.ctx.DataDir)
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// syncSelected handles the Sync to PostgreSQL menu item.
func (a *App) syncSelected(i *menu.Item) {
	modal := tview.NewModal().SetText("Syncing to PostgreSQL...")
	a.mainPages.AddPage("modal", modal, true, true)
	a.SetFocus(modal)

	go func() {
		err := a.m.Sync()

		a.QueueUpdateDraw(func() {
			if err != nil {
				modal.SetText("Sync failed: " + err.Error())
			} else {
				modal.SetText("Sync completed successfully")
			}
			modal.AddButtons([]string{"OK"})
			modal.SetDoneFunc(func(int, string) {
				a.mainPages.RemovePage("modal")
			})
			a.SetFocus(modal)
		})
	}()
}

// supplierMappingSelected handles the Supplier Mapping menu item.
func (a *App) supplierMappingSelected(i *menu.Item) {
	modal := tview.NewModal().SetText("Loading sessions...")
	a.mainPages.AddPage("modal", modal, true, true)
	a.SetFocus(modal)

	go func() {
		// Ensure DB is started (same pattern as RefreshSession)
		if a.m.db.GetDB() == nil {
			if err := a.m.db.Start(); err != nil {
				a.QueueUpdateDraw(func() {
					modal.SetText("Failed to start database: " + err.Error())
					modal.AddButtons([]string{"OK"})
					modal.SetDoneFunc(func(int, string) { a.mainPages.RemovePage("modal") })
					a.SetFocus(modal)
				})
				return
			}
		}
		resp, err := a.m.db.GetSessions("", 0, 0)
		a.QueueUpdateDraw(func() {
			a.mainPages.RemovePage("modal")
			if err != nil {
				a.showError(fmt.Errorf("failed to load sessions: %v", err))
				return
			}
			a.showSupplierMappingMenu(resp.Items)
		})
	}()
}

// showSupplierMappingMenu shows a submenu listing all sessions with their mapping status.
func (a *App) showSupplierMappingMenu(sessions []*model.Session) {
	subMenu := menu.NewSubMenu("Supplier Mapping")
	mappings := a.ctx.GetSupplierMappings()

	for idx, sess := range sessions {
		tag := "[unmapped]"
		if sid, ok := mappings[sess.UserName]; ok {
			tag = fmt.Sprintf("[%s]", sid)
		}
		name := fmt.Sprintf("%s (%s) %s", sess.NickName, sess.UserName, tag)

		session := sess
		currentSID := mappings[sess.UserName]
		subMenu.AddItem(&menu.Item{
			Index:       idx + 1,
			Name:        name,
			Description: "Edit supplier mapping",
			Selected: func(*menu.Item) {
				a.showSupplierMappingForm(session, currentSID)
			},
		})
	}

	if len(sessions) == 0 {
		subMenu.AddItem(&menu.Item{
			Index:       1,
			Name:        "No sessions available",
			Description: "Decrypt data first to load sessions",
		})
	}

	a.mainPages.AddPage("submenu", subMenu, true, true)
	a.SetFocus(subMenu)
}

// showSupplierMappingForm shows a form to edit the supplier mapping for a session.
func (a *App) showSupplierMappingForm(session *model.Session, currentSupplierID string) {
	formView := form.NewForm(fmt.Sprintf("Mapping: %s", session.NickName))

	tempSupplierID := currentSupplierID

	formView.AddInputField("Supplier ID", tempSupplierID, 0, nil, func(text string) {
		tempSupplierID = text
	})

	formView.AddButton("Save", func() {
		if tempSupplierID != "" {
			a.ctx.SetSupplierMapping(session.UserName, tempSupplierID)
		}
		a.mainPages.RemovePage("submenu2")
		a.mainPages.RemovePage("submenu")
		a.showInfo(fmt.Sprintf("Mapped %s → %s", session.UserName, tempSupplierID))
	})

	formView.AddButton("Clear", func() {
		a.ctx.RemoveSupplierMapping(session.UserName)
		a.mainPages.RemovePage("submenu2")
		a.mainPages.RemovePage("submenu")
		a.showInfo(fmt.Sprintf("Cleared mapping for %s", session.UserName))
	})

	formView.AddButton("Cancel", func() {
		a.mainPages.RemovePage("submenu2")
	})

	a.mainPages.AddPage("submenu2", formView, true, true)
	a.SetFocus(formView)
}

// selectAccountSelected 处理切换账号菜单项的选择事件
func (a *App) selectAccountSelected(i *menu.Item) {
	// 创建子菜单
	subMenu := menu.NewSubMenu("Switch Account")

	// 添加微信进程
	instances := a.m.wechat.GetWeChatInstances()
	if len(instances) > 0 {
		// 添加实例标题
		subMenu.AddItem(&menu.Item{
			Index:       0,
			Name:        "--- WeChat Processes ---",
			Description: "",
			Hidden:      false,
			Selected:    nil,
		})

		// 添加实例列表
		for idx, instance := range instances {
			description := fmt.Sprintf("Version: %s Dir: %s", instance.FullVersion, instance.DataDir)

			name := fmt.Sprintf("%s [%d]", instance.Name, instance.PID)
			if a.ctx.Current != nil && a.ctx.Current.PID == instance.PID {
				name = name + " [Current]"
			}

			// 创建菜单项
			instanceItem := &menu.Item{
				Index:       idx + 1,
				Name:        name,
				Description: description,
				Hidden:      false,
				Selected: func(instance *wechat.Account) func(*menu.Item) {
					return func(*menu.Item) {
						if a.ctx.Current != nil && a.ctx.Current.PID == instance.PID {
							a.mainPages.RemovePage("submenu")
							a.showInfo("Already the current account")
							return
						}

						modal := tview.NewModal().SetText("Switching account...")
						a.mainPages.AddPage("modal", modal, true, true)
						a.SetFocus(modal)

						// 在后台执行切换操作
						go func() {
							err := a.m.Switch(instance, "")

							// 在主线程中更新UI
							a.QueueUpdateDraw(func() {
								a.mainPages.RemovePage("modal")
								a.mainPages.RemovePage("submenu")

								if err != nil {
									// 切换失败
									a.showError(fmt.Errorf("切换账号失败: %v", err))
								} else {
									// 切换成功
									a.showInfo("切换账号成功")
									// 更新菜单状态
									a.updateMenuItemsState()
								}
							})
						}()
					}
				}(instance),
			}
			subMenu.AddItem(instanceItem)
		}
	}

	// 添加历史账号
	if len(a.ctx.History) > 0 {
		// 添加历史账号标题
		subMenu.AddItem(&menu.Item{
			Index:       100,
			Name:        "--- History Accounts ---",
			Description: "",
			Hidden:      false,
			Selected:    nil,
		})

		// 添加历史账号列表
		idx := 101
		for account, hist := range a.ctx.History {
			// 创建一个账号描述
			description := fmt.Sprintf("版本: %s 目录: %s", hist.FullVersion, hist.DataDir)

			// 标记当前选中的账号
			name := account
			if name == "" {
				name = filepath.Base(hist.DataDir)
			}
			if a.ctx.DataDir == hist.DataDir {
				name = name + " [Current]"
			}

			// 创建菜单项
			histItem := &menu.Item{
				Index:       idx,
				Name:        name,
				Description: description,
				Hidden:      false,
				Selected: func(account string) func(*menu.Item) {
					return func(*menu.Item) {
						if a.ctx.Current != nil && a.ctx.DataDir == a.ctx.History[account].DataDir {
							a.mainPages.RemovePage("submenu")
							a.showInfo("Already the current account")
							return
						}

						modal := tview.NewModal().SetText("Switching account...")
						a.mainPages.AddPage("modal", modal, true, true)
						a.SetFocus(modal)

						// 在后台执行切换操作
						go func() {
							err := a.m.Switch(nil, account)

							// 在主线程中更新UI
							a.QueueUpdateDraw(func() {
								a.mainPages.RemovePage("modal")
								a.mainPages.RemovePage("submenu")

								if err != nil {
									// 切换失败
									a.showError(fmt.Errorf("切换账号失败: %v", err))
								} else {
									// 切换成功
									a.showInfo("切换账号成功")
									// 更新菜单状态
									a.updateMenuItemsState()
								}
							})
						}()
					}
				}(account),
			}
			idx++
			subMenu.AddItem(histItem)
		}
	}

	// 如果没有账号可选择
	if len(a.ctx.History) == 0 && len(instances) == 0 {
		subMenu.AddItem(&menu.Item{
			Index:       1,
			Name:        "No accounts available",
			Description: "No WeChat process or history account detected",
			Hidden:      false,
			Selected:    nil,
		})
	}

	// 显示子菜单
	a.mainPages.AddPage("submenu", subMenu, true, true)
	a.SetFocus(subMenu)
}

// showModal 显示一个模态对话框
func (a *App) showModal(text string, buttons []string, doneFunc func(buttonIndex int, buttonLabel string)) {
	modal := tview.NewModal().
		SetText(text).
		AddButtons(buttons).
		SetDoneFunc(doneFunc)

	a.mainPages.AddPage("modal", modal, true, true)
	a.SetFocus(modal)
}

// showError 显示错误对话框
func (a *App) showError(err error) {
	a.showModal(err.Error(), []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.mainPages.RemovePage("modal")
	})
}

// showInfo 显示信息对话框
func (a *App) showInfo(text string) {
	a.showModal(text, []string{"OK"}, func(buttonIndex int, buttonLabel string) {
		a.mainPages.RemovePage("modal")
	})
}
