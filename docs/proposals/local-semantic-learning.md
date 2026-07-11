# 本地语义辅助与规则学习方案

> 状态：提案（Proposal），已确认决策，待实施
> 决策记录：L5 外部 AI 延后；源 B 受信目录为 `var/corpus/`；规则草案启用试用期机制；源 C 权重调整上限 ±3 分/次
> 关联：M5 路线图保留项；K-008 规则优先级；K-009 隐私与 AI 边界；K-000 原则 7（AI 只建议，不独立执行）

## 1. 目标与边界

### 目标
把当前硬编码在 `dircontext/classifier.go` 的目录角色信号、敏感词、业务锚点模式，升级为**可学习、可审计、可回滚**的规则系统，让整理建议贴合用户实际的目录命名习惯与行业语境。

### 必须遵守的边界（不可妥协）
- **本地优先**：学习在本地完成，本地数据库，不上传文件名或内容（K-009）。
- **外部 AI 默认关闭**：任何调用外部模型的能力必须显式开启，且只发送脱敏后的抽象特征，绝不发送原始路径、文件名、内容片段。
- **AI 只建议**：学习产物一律是"规则草案"，状态为 DRAFT，必须人工审批后才能生效；生效规则也不独立执行破坏性操作（K-000 原则 7）。
- **保护优先**：学习产出的规则永远低于系统保护与用户保护规则（K-008）；冲突显式进入复核，不静默选择。
- **可审计可回滚**：每条规则的来源、学习批次、审批人、生效时间、命中次数必须可查；规则可禁用、可回滚到上一版本。

### 非目标（本期不做）
- 不做感知哈希、图像/音频内容相似度（K-006 渐进式分析之外的层级）。
- 不做在线联网学习；不做模型微调。
- 不让规则自动触发删除/移动；规则只影响 planner 的建议与评分。

---

## 2. 三类学习输入源

### 源 A：本地数据遍历学习（统计归纳）
**输入**：scan 产出的索引（`file_instances` + `directory_contexts`），只读。
**学什么**：
- 目录命名频率：用户高频使用的目录名（如 `素材`、`交付件`、`甲方文档`）→ 候选角色信号。
- 命名约定模式：路径中反复出现的项目代号格式（如 `ABC-2024-XX`）→ 业务锚点正则草案。
- 目录角色分布：某类目录下文件数量、平均大小、修改时间跨度 → 辅助判断临时/归档/工作。
- 时间窗口聚类：某时间段集中写入的文件 → 候选资产组边界。

**怎么学**：
1. 遍历已入库的 `file_instances`，按目录聚合统计。
2. 计算每个目录名出现的频次、与已有角色信号的匹配率。
3. 频次 ≥ 阈值且未被现有规则覆盖的目录名，生成"候选信号"。
4. 对项目代号格式，从路径段中提取 `^[A-Z]+[-_]\d{2,}` 类模式，统计命中数量，高频模式生成锚点正则草案。

**隐私处理**：统计只记录"目录名 → 频次"，不记录完整路径；项目代号正则只保留模式（如 `[A-Z]{3}-\d{4}-\d{2}`），不保留具体代号值。敏感目录（命中 sensitiveTerms）的统计结果丢弃，不进入学习样本。

### 源 B：行业专项资料学习（用户提供）
**输入**：用户显式提供的资料文件（PDF/DOCX/MD/TXT），放在受信目录（如 `~/.nas-governance/corpus/`）。
**学什么**：
- 行业术语词典：归档规范、文件分类标准中的名词（如建筑行业的"竣工图"、"施工日志"）。
- 角色映射建议：术语 → 目录角色的映射草案（如"竣工图" → RoleFormalArchive）。
- 敏感词扩充：行业特有的敏感词（如医疗行业的"病历"、"处方"）。

**怎么学**：
1. 用户提供资料路径，系统读取资料文本（PDF 用 M4 的页数提取；DOCX/MD/TXT 直接读取；不调用 OCR）。
2. 分词 + 词频统计，提取高频名词短语（n-gram）。
3. 与现有 `roleSignals` 术语比对，未覆盖的高频词生成"候选术语"。
4. 用户在 CLI 中确认每个候选术语的角色映射（交互式或批量 YAML 导出）。

**隐私处理**：资料文件由用户主动放入受信目录，视为已授权；但学习产物（规则）不保留资料原文，只保留提取的术语和映射。资料路径不写入审计日志。

### 源 C：决策反馈学习（在线增量）
**输入**：用户对 plan 的 approve/reject/edit 决策记录（`operation_logs` + `operation_plans`）。
**学什么**：
- 用户保留哪一份副本、删除哪一份 → 校准 `ScoreRetention` 的权重。
- 用户反复拒绝某类建议 → 该类规则的置信度下降。
- 用户手动编辑 action（如把 REVIEW 改成 QUARANTINE）→ 强化该路径模式下的动作倾向。

**怎么学**：
1. 统计历史 plan 中，每个 `retain_path` 的特征（角色、深度、mtime）与评分排名的吻合度。
2. 若用户频繁保留评分非最高的副本，生成"权重调整建议"（如 Stability 权重 +5）。
3. 拒绝率 ≥ 阈值的规则，置信度降级，超过下限则标记为"待复核"。

**隐私处理**：只读取 plan 的结构化字段（action type、retain_score、evidence），不读取路径；审计日志已脱敏（K-009）。

---

## 3. 规则表示与优先级

### 规则结构（YAML）
```yaml
# 存储在 rules 表的 definition_yaml 字段
id: signal-archive-term-jungong
version: 2
priority: 60          # K-008 优先级：保护(100) > 角色(60) > 文件类型(50) > 去重(40) > 清理(20)
enabled: true
source: learned        # learned | builtin | user
batch: "learn-2026-07-11-001"
confidence: 0.82
status: approved       # draft | approved | disabled
approved_at: "2026-07-11T10:00:00Z"
expires_at: null      # 可选：规则自动失效时间
type: directory_signal
match:
  segment_contains: "竣工图"   # 匹配条件
effect:
  role: formal_archive
  authority: 90
  protected: true
evidence:
  - "来源：源 B 行业资料学习，批次 learn-2026-07-11-001"
  - "命中频次：12 次目录出现"
  - "置信度：0.82"
```

### 优先级层次（K-008）
学习产出的规则**只能落在 priority ≤ 60 的层次**，永远低于系统保护（100）和用户保护（90）：

| 层次 | priority | 来源 | 冲突行为 |
|------|----------|------|----------|
| 系统保护 | 100 | builtin 硬编码 | 不可覆盖 |
| 用户保护 | 90 | 用户显式声明 | 不可被学习规则覆盖 |
| 目录角色 | 60 | builtin + learned | 冲突 → 复核 |
| 文件类型 | 50 | builtin + learned | 冲突 → 复核 |
| 去重评分 | 40 | learned | 调整权重，不直接决策 |
| 默认保守 | 0 | 内置兜底 | 最终回退 |

### 冲突处理
当学习规则与 builtin 规则冲突（同一目录名匹配不同角色）：
1. builtin 优先，学习规则不生效。
2. 记录冲突事件到 `rule_conflicts` 表（新增），供人工复核。
3. 不静默选择删除（K-008）。

---

## 4. 学习引擎架构

### 包结构
```
internal/learning/
  ├── corpus.go        # 源 B：资料读取与术语提取
  ├── stats.go         # 源 A：本地数据统计归纳
  ├── feedback.go      # 源 C：决策反馈分析
  ├── rule.go          # 规则草案生成（统一出口）
  └── rule_test.go
```

### 学习流水线
```
scan 索引 ─┐
           ├─→ stats.go ──→ 候选信号 ─┐
受信资料 ──┤                         ├─→ rule.go ──→ 规则草案(DRAFT) ──→ store ──→ 人工审批
           ├─→ corpus.go ─→ 候选术语 ─┘
历史决策 ──┘
           └─→ feedback.go ─→ 权重调整建议 ──→ 规则草案(DRAFT)
```

### 规则版本与失效
- 每次学习生成一个 `batch_id`（如 `learn-2026-07-11-001`），批次内所有规则共享。
- 规则生效时写入 `rule_version`；classifier 重新分类时用 `rule_version` 判断是否需要刷新已缓存的 `directory_contexts`。
- 规则可被禁用（`enabled=false`），不删除，保留审计记录。
- 支持按 `batch_id` 整批回滚（禁用该批次所有规则）。

---

## 5. 隐私保护设计

### 数据流脱敏
| 阶段 | 原始数据 | 保留在样本中 | 进入规则 |
|------|----------|-------------|---------|
| 路径 | `/data/PRJ-2024-001/合同.pdf` | 否 | 只保留"命中项目代号模式" |
| 目录名 | `合同` | 频次统计保留 | 作为术语候选 |
| 文件名 | `合同_甲方.pdf` | 否 | 不学习文件名 |
| 内容 | 不读取 | 不读取 | 不读取 |

### 外部 AI 调用（可选，默认关闭）
若用户显式开启 `--ai-assist`：
- 只发送脱敏后的**抽象特征**：`{category: "document", frequency: 12, co_occurrence: ["归档", "已结项"]}`。
- 绝不发送：路径、文件名、内容片段、项目代号原值。
- AI 输出只能是"规则草案建议"，仍需人工审批。
- 调用记录写入审计日志（记录调用的端点、时间、token 数，不记录请求体）。

### 敏感目录豁免
命中 `sensitiveTerms` 的目录，其统计结果**丢弃**，不进入学习样本。这防止学习出"财务目录里的文件应该删除"之类的危险规则。

---

## 6. 人工确认管线

### 规则生命周期
```
学习产出 → DRAFT（草案）→ 人工审批 → PROBATION（试用期）→ APPROVED（正式生效）→ 可禁用 → DISABLED
                                    ↓ 审批拒绝                ↓ 试用期内被 disable 或命中冲突
                                  REJECTED（归档）           → DISABLED（回滚）
```
- **PROBATION（试用期）**：审批后不立即正式生效，先观察 N 天（默认 7 天，可配置）。试用期内规则参与分类但标记为"试用中"，命中记录写入 `rule_hits`，若试用期内被 disable 或命中冲突则回滚为 DISABLED；试用期满无异常则自动转为 APPROVED。
- 试用期规则在评分中带 `confidence * 0.8` 折扣，避免新规则过早主导决策。

### CLI 交互
- `nas-governance learn --source=stats`：触发源 A 学习，输出规则草案 JSON。
- `nas-governance learn --source=corpus --corpus-dir=var/corpus/`：触发源 B 学习（默认受信目录 `var/corpus/`，可覆盖）。
- `nas-governance rules list --status=draft`：列出待审批规则。
- `nas-governance rules approve --batch=learn-2026-07-11-001`：审批整批。
- `nas-governance rules reject --id=signal-xxx`：拒绝单条。
- `nas-governance rules disable --batch=learn-2026-07-11-001`：整批禁用（回滚）。

### 安全约束
- 审批后的规则**只影响 planner 的建议和评分**，不直接触发文件系统操作。
- 规则产生的 action 仍然走完整的 M3 安全管线（stale-check → approve → execute → verify）。
- 规则命中记录写入 `rule_hits` 表（新增），供后续评估规则有效性。

---

## 7. 持久化（schema 扩展）

```sql
-- 已有 rules 表扩展字段
ALTER TABLE rules ADD COLUMN source TEXT DEFAULT 'builtin';     -- builtin | learned | user
ALTER TABLE rules ADD COLUMN batch_id TEXT;
ALTER TABLE rules ADD COLUMN confidence REAL DEFAULT 1.0;
ALTER TABLE rules ADD COLUMN status TEXT DEFAULT 'approved';    -- draft | probation | approved | disabled | rejected
ALTER TABLE rules ADD COLUMN approved_at TEXT;
ALTER TABLE rules ADD COLUMN expires_at TEXT;

-- 新增：规则冲突记录
CREATE TABLE IF NOT EXISTS rule_conflicts (
  id INTEGER PRIMARY KEY,
  rule_id_a TEXT NOT NULL REFERENCES rules(id),
  rule_id_b TEXT NOT NULL REFERENCES rules(id),
  path_hash TEXT NOT NULL,           -- 路径哈希，不存原路径（K-009）
  resolved INTEGER DEFAULT 0,
  created_at TEXT NOT NULL
);

-- 新增：规则命中统计
CREATE TABLE IF NOT EXISTS rule_hits (
  id INTEGER PRIMARY KEY,
  rule_id TEXT NOT NULL REFERENCES rules(id),
  hit_count INTEGER DEFAULT 0,
  last_hit_at TEXT,
  UNIQUE(rule_id)
);

-- 新增：学习批次记录
CREATE TABLE IF NOT EXISTS learning_batches (
  id TEXT PRIMARY KEY,              -- learn-2026-07-11-001
  source TEXT NOT NULL,             -- stats | corpus | feedback
  started_at TEXT NOT NULL,
  completed_at TEXT,
  rule_count INTEGER DEFAULT 0,
  status TEXT DEFAULT 'running'     -- running | completed | failed
);
```

---

## 8. 实现阶段拆分

### 阶段 L1：规则基础设施（先行）
- 扩展 `rules` 表字段（source/batch/confidence/status）。
- 新增 `rule_conflicts` / `rule_hits` / `learning_batches` 表。
- 重构 `classifier.Classify`：从硬编码 `roleSignals` 改为读取 builtin + learned 规则，builtin 规则从内置常量加载，learned 规则从 DB 加载。
- CLI `rules list/approve/reject/disable` 子命令。
- **验收**：现有测试全过（builtin 规则行为不变）；规则可审批、可禁用、可按批次回滚。

### 阶段 L2：源 A 本地统计学习
- `internal/learning/stats.go`：遍历索引，统计目录名频次、项目代号模式。
- `internal/learning/rule.go`：生成规则草案，写入 DB（status=draft）。
- CLI `learn --source=stats`。
- 隐私：敏感目录豁免、路径脱敏。
- **验收**：学习产出的规则草案经人工审批后，能让 classifier 识别用户实际使用的目录名；冲突进入复核；敏感目录不被学习。

### 阶段 L3：源 B 行业资料学习
- `internal/learning/corpus.go`：读取受信目录资料，分词，术语提取。
- 术语 → 角色映射的交互式确认。
- CLI `learn --source=corpus --corpus-dir=...`。
- **验收**：资料中的行业术语能被提取为候选规则；用户确认后生效；资料原文不进入规则或日志。

### 阶段 L4：源 C 决策反馈学习
- `internal/learning/feedback.go`：分析历史 plan 决策。
- 权重调整建议（调整 `ScoreRetention` 的权重，作为规则草案），单次调整上限 ±3 分，避免大幅漂移。
- **验收**：反馈学习产出的权重调整经审批后影响后续 planner 的评分；拒绝率高的规则置信度下降；单次权重变化不超过 ±3 分。

### 阶段 L5（可选）：外部 AI 辅助
- `--ai-assist` 显式开启，只发送脱敏抽象特征。
- AI 输出规则草案，仍走人工审批。
- **验收**：不开启时无任何外部调用；开启时只发送抽象特征，不发路径/文件名/内容。

---

## 9. 风险与缓解

| 风险 | 缓解 |
|------|------|
| 学习产出错误规则导致误删 | 规则只影响建议和评分，不直接执行；执行仍走 M3 全管线 |
| 敏感路径泄露 | 源 A 统计丢弃敏感目录；规则只存模式不存原值；审计日志脱敏 |
| 规则膨胀 | 规则命中统计（`rule_hits`）；长期未命中规则提示清理 |
| 外部 AI 越界 | 默认关闭；开启时只发抽象特征；输出仍为草案需审批 |
| 学习规则覆盖保护 | priority 层次限制，learned 规则 ≤ 60，永远低于保护规则 |

---

## 10. 与现有系统的集成点

- **classifier**：从硬编码改为读取 builtin + learned 规则；`rule_version` 驱动 context 刷新。
- **planner**：`ScoreRetention` 的权重可被 learned 规则调整（源 C）。
- **store**：新增规则相关表的读写方法。
- **executor**：不受直接影响；规则产出的 plan 仍走 M3 管线。
- **CLI**：新增 `learn` 和 `rules` 子命令组。

---

## 附：决策清单（已确认）

1. 阶段优先级：L1 → L2 → L3 → L4，L5 延后。
2. 源 B 资料学习的受信目录默认路径：`var/corpus/`（项目内）。
3. 规则草案启用试用期机制：审批后进入 PROBATION，默认 7 天无异常后转 APPROVED。
4. 外部 AI 辅助（L5）延后，本期只做本地学习。
5. 源 C 权重调整上限：单次 ±3 分。
