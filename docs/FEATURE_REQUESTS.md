# Feature Requests

AI エージェント（Claude Code 等）から UniForge を活用する中で感じた機能要望のリスト。
確定したバックログではなく、検討・議論のための要望集。

---

## ステータス一覧

| # | 要望 | 状態 | 備考 |
|---|------|------|------|
| 1 | `compile` コマンド | **未着手** | テスト不要なコンパイル確認の需要が高い |
| 2 | `test --list` | **保留** | Unity バッチモードの `RetrieveTestList` コールバック問題あり |
| 3 | `meta generate` | **未着手** | GUID 生成ルールの調査が必要 |
| 4 | `project info` | ✅ **実装済み** | `uniforge project info` / `--json` 対応 |
| 5 | テスト結果 XML 制御 | ✅ **実装済み** | `--results-dir` で出力先指定可能 |
| 6 | `--filter` 拡張 | ✅ **対応不要** | Unity が正規表現・セミコロン区切りを既にサポート。ヘルプに使用例追記済み |
| 7 | テスト結果サマリ | ✅ **実装済み** | NUnit XML パースで Total/Passed/Failed/Skipped/Duration を表示 |
| 8 | `restart --force` | ✅ **実装済み** | `--force` フラグで SIGKILL 後に再起動 |

---

## 未着手の要望

### 1. `uniforge compile` - コンパイルのみ実行

**背景**: C# コードを変更した後、テストを実行するほどではないが「コンパイルが通るか」だけ確認したい場面が頻繁にある。現状は `uniforge test` でテストごと実行するか、`uniforge run -- -executeMethod ...` でカスタムメソッドを呼ぶしかない。

**要望**:
```bash
# コンパイルのみ実行（テストは走らせない）
uniforge compile <project-path>

# コンパイル結果のみを返す（成功/失敗 + エラー詳細）
uniforge compile <project-path> --json
```

**ユースケース**:
- namespace やクラス名のリネーム後、テスト前にまずコンパイルだけ確認
- asmdef の参照変更後のコンパイルチェック
- 複数ファイルの大規模編集中の中間確認

---

### 3. `uniforge meta generate` - .meta ファイル生成

**背景**: エージェントが新しい C# ファイルを作成した場合、対応する .meta ファイルが存在しない。`uniforge meta check` で検出はできるが、生成はできない。現状は Unity エディタを起動して自動生成を待つしかない。

**要望**:
```bash
# 不足している .meta ファイルを生成
uniforge meta generate <project-path>

# 特定のファイルに対して .meta を生成
uniforge meta generate <project-path> --path Assets/Editor/NewFile.cs
```

**備考**: GUID の生成ルールなど Unity 内部仕様との整合性が必要。実現が難しければ、「.meta が不足しているファイル一覧を表示し、Unity エディタでの自動生成を促す」だけでも有用。

---

## 保留中の要望

### 2. `uniforge test --list` - テスト一覧表示

**背景**: `--filter` オプションでテストを絞り込む際、指定すべきテストクラス名やメソッド名がわからないことがある。テスト一覧を事前に確認できると便利。

**要望**:
```bash
# EditMode テストの一覧を表示
uniforge test <project-path> --platform editmode --list

# PlayMode テストの一覧を表示
uniforge test <project-path> --platform playmode --list
```

**保留理由**: Unity の `TestRunnerApi.RetrieveTestList()` はバッチモード（`-executeMethod`）でコールバックが呼ばれない既知の問題がある。プロジェクト側にカスタム C# スクリプトを配置する必要があり、侵入的な実装になる。

**代替案**: テスト結果 XML（`--results` で取得可能）からテストクラス・メソッド一覧を事後的に確認する。

---

## 実装済みの要望

### 4. `uniforge project info` - プロジェクト情報表示 ✅

```bash
uniforge project info <project-path>    # テキスト出力
uniforge project info <project-path> --json  # JSON 出力
uniforge project info my-project         # Unity Hub プロジェクト名でも検索可能
```

Unity バージョン、Packages（manifest.json）、Assembly Definitions を表示。

---

### 5. テスト結果 XML の出力制御 ✅

```bash
# 出力先ディレクトリを指定
uniforge test <project-path> --platform editmode --results-dir /tmp/test-results/
```

`--results-dir` で出力先を指定可能。ファイル名は `TestResults-{platform}.xml` で自動生成。

---

### 6. `uniforge test --filter` のワイルドカード/正規表現サポート ✅

Unity の `-testFilter` が正規表現・セミコロン区切りを既にサポートしており、UniForge はフィルタ文字列をそのまま引き渡している。追加実装は不要。

```bash
# クラス名で部分一致
uniforge test --platform editmode --filter MyTestClass

# 複数クラスをセミコロン区切り（スペース不可）
uniforge test --platform editmode --filter "ClassA;ClassB"

# 名前空間で正規表現フィルタ
uniforge test --platform editmode --filter "^MyNamespace\."

# テストを除外
uniforge test --platform editmode --filter "!SlowTestClass"
```

---

### 7. テスト結果のサマリ出力 ✅

テスト結果 XML が存在する場合（`--results` または `--results-dir` 指定時）、テスト完了後にサマリを表示:
```
=== Test Results ===
Total: 328  Passed: 328  Failed: 0  Skipped: 0
Duration: 0.11s
✓ Result: PASSED
```

テスト失敗時もサマリを表示する。XML が存在しない場合は従来通り `✓ Tests completed successfully` を表示。

---

### 8. `uniforge restart --force` - 強制再起動 ✅

```bash
uniforge restart <project-path> --force
```

SIGKILL でプロセスを強制終了してから再起動する。
