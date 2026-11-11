// Package domain は Confluence のページ操作で使う型とエラーを定義する。
// 他のパッケージに依存しない。
package domain

import "time"

type PageID string

type Version struct {
	Number    int
	Message   string
	CreatedAt time.Time
	AuthorID  string
	MinorEdit bool
}

// Page の Body は storage 形式。本文を取得しなかった場合は空になる。
type Page struct {
	ID       PageID
	Title    string
	Status   string
	SpaceID  string
	ParentID string
	WebURL   string
	Version  Version
	Body     string
}

type PageSummary struct {
	ID      PageID
	Title   string
	Status  string
	SpaceID string
}

// Ancestor の Type は page・folder など。祖先にはフォルダーも含まれる。
type Ancestor struct {
	ID    PageID
	Type  string
	Title string
}

type Space struct {
	ID   string
	Key  string
	Name string
}

type Label struct {
	ID     string
	Name   string
	Prefix string
}

// SearchHit の SpaceID は、検索 API が返さないため、タイトル検索のときだけページ API で埋める。それ以外では空。
type SearchHit struct {
	ID           PageID
	Title        string
	Type         string
	SpaceKey     string
	SpaceID      string
	Excerpt      string
	URL          string
	LastModified time.Time
}
