// Package convert は storage 形式と Markdown の相互変換で共有する対応表を提供する。
package convert

// PanelToAlert は Confluence のパネルマクロ名から GitHub Alerts の種類への対応。
// Confluence の note マクロは黄、warning マクロは赤で表示されるため、色が合う WARNING（黄）・CAUTION（赤）に対応させる。
var PanelToAlert = map[string]string{"info": "NOTE", "tip": "TIP", "note": "WARNING", "warning": "CAUTION"}

// AlertToPanel は GitHub Alerts の種類から Confluence のパネルマクロ名への対応。
// IMPORTANT に対応するパネルがないため、info に対応させる。
var AlertToPanel = map[string]string{"NOTE": "info", "IMPORTANT": "info", "TIP": "tip", "WARNING": "note", "CAUTION": "warning"}
