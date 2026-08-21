# ADR（Architecture Decision Record）

設計判断の記録を置くディレクトリ。ファイル名の連番とタイトルがそのまま一覧になるため、
別途の索引は持たない。

- ファイル名: `NNNN-kebab-case-の英語タイトル.md`（4 桁ゼロ埋め、欠番も再利用もしない）
- ステータス: `Proposed` / `Accepted` / `Superseded by ADR-NNNN`
- 決定を変えるときは既存の ADR を書き換えず、新しい ADR を追加して古い方を supersede する

何を ADR にするか、各項目に何を書くかは [.claude/skills/adr/SKILL.md](../../.claude/skills/adr/SKILL.md)、
雛形は [.claude/skills/adr/assets/template.md](../../.claude/skills/adr/assets/template.md) を参照。
