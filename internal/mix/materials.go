package mix

// Materials are the unrelated things a seed idea is forced together with.
// They are drawn by lot, not chosen by a model: a model asked to be
// original reaches for the same handful of combinations every time.
var Materials = []string{
	// things to read
	"CSV", "JSON", "ログの行", "日付", "UNIX 時刻", "URL", "色コード", "IP アドレス",
	"ファイルサイズ", "環境変数ファイル（.env）", "Markdown", "正規表現", "絵文字",
	"全角と半角", "文字の表示幅", "和暦", "曜日", "干支", "ローマ数字", "モールス信号",
	"時間の長さ（1h30m など）", "単位（長さ・重さ・温度）", "金額の表記", "UUID",
	"ハッシュ値", "Base64", "素数", "乱数", "合言葉", "締め切り",
	// ways to show
	"QR コード", "スパークライン", "横棒グラフ", "ヒストグラム", "罫線の表", "進捗バー",
	"大きな ASCII 文字", "色つき表示", "カレンダー", "ツリー表示", "差分", "1 行の要約",
	"終了コードでの合否", "点字のドット絵",
	// things to do
	"数える", "そろえる", "並べ替える", "分割する", "間引く", "まとめる", "検査する", "逆にする",
}

// Seeds are starting ideas for when the shelf is empty and the user gave
// none: small tools of the kind the line is known to build.
var Seeds = []string{
	"QR コードを端末に表示する",
	"CSV を Markdown の表にする",
	"文字数と表示幅を数える",
	"テキストを指定幅で折り返す",
	"UNIX 時刻と日時を相互に変換する",
	"数値の列をスパークラインで表示する",
	"西暦を和暦に変換する",
}
