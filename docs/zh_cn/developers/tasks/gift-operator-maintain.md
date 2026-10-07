# 开发手册 - 赠送干员礼物维护文档

本文说明 `GiftOperator` 的文件分布与收礼、送礼两个独立阶段。
该文档更新于 2026 年 10 月 7 日。

## 文件路径

| 路径 | 作用 |
| ------------------------------------------------------------------- | ----------------------------- |
| `assets/interface.json` | 任务挂载（`dijiang_ship` 组） |
| `assets/tasks/GiftOperator.json` | 任务入口与界面选项 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorMain.json` | 入口、帝江号定位 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorNavigation.json` | 寻路与联络台接触 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorContact.json` | 联络界面选人 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorReceiveFlow.json` | 收礼选人、领取与每日五份计数 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorGiftFlow.json` | 对话中的送礼 |
| `agent/go-service/giftoperator/` | 单人候选识别与送礼人数、成功/排除身份记录 |
| `tests/GiftOperator/test_gift_ui_status.json` | 送礼界面信赖、每日上限与预选提示识别测试 |
| `assets/resource/pipeline/GiftOperator/GiftOperatorBagFull.json` | 背包已满处理 |
| `assets/resource/pipeline/GiftOperator/Operator/Operator.json` | 收礼阶段的干员识别与名称白名单 |
| `assets/resource/image/GiftOperator/` | Win32 识别图片 |
| `assets/resource_adb/image/GiftOperator/` | ADB 识别图片 |
| `assets/resource_adb/pipeline/GiftOperator/` | ADB Pipeline 镜像 |
| `tools/gift_operator/fill_gift_operator_green_box.py` | 干员头像 green_mask 格式化 |
| `assets/locales/interface/*.json` | 任务、选项与干员名称文案 |

## 新增干员时需改的路径

新增一名干员时，至少需同步以下 7 处（`<Name>` 为干员标识，与模板文件名、option case 名保持一致）：

| # | 路径 | 说明 |
| --- | -------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------- |
| 1 | `assets/resource/image/GiftOperator/Operators/<Name>.png` | Win32 干员头像模板；入库前须用 `tools/gift_operator/fill_gift_operator_green_box.py` 处理 |
| 2 | `assets/resource_adb/image/GiftOperator/Operators/<Name>.png` | ADB 干员头像模板；同上处理 |
| 3 | `assets/tasks/GiftOperator.json` → `SelectOperator` | 新增 case，在 UI 提供送礼对象，并配置 `GiftOperatorSendCandidate.attach.templates` 与送礼名称白名单 `GiftOperatorName` |
| 4 | `assets/resource/pipeline/GiftOperator/Operator/Operator.json` | 收礼阶段识别干员头像，并覆盖独立的名称白名单 `GiftOperatorReceiveName` |
| 5 | `assets/resource/pipeline/GiftOperator/GiftOperatorReceiveFlow.json` → `GiftOperatorSelectGiftOp.next` | 在收礼选人节点的 `next` 数组追加 `GiftOperatorSelect_<Name>`，否则该干员节点不会被触发 |
| 6 | `assets/resource/pipeline/GiftOperator/GiftOperatorGiftFlow.json` → `GiftOperatorSendCandidate.attach.operators` | 追加 `GiftOperatorSelect_<Name>`，让送礼候选器复用该干员的头像与五语言姓名 |
| 7 | `assets/locales/interface/*.json` → `operator.<Name>` | 各语言干员显示名称 |

## 路线一：默认（先收礼，再送礼）

对应选项「只收礼物」关闭。任务入口 `GiftOperatorMain` 通过 `SubTask` 顺序执行 `StashBackpackSubTask`、`GiftOperatorReceiveMain`、`GiftOperatorSendMain`。收礼完成后，才按配置的赠送对象与赠送数量执行送礼。

送礼对象选项 `SelectOperator` 提供「任意干员」、各指定干员和「只向信赖未满的干员送礼」（`AnyNonMaxTrust`）。「送礼人数」（`GiftOperatorCount`）为独立的正整数输入，默认 `1`；「赠送数量」（`GiftCount`）是每位干员的礼物数。这些配置只影响送礼阶段，不影响前面的每日五份收礼。

1. 任务开始前存放背包。
2. 执行独立收礼阶段，逐一领取每日五份礼物；若当天已领取过部分礼物，扫描完剩余礼物后结束该阶段。具体流程见[路线二](#路线二只收礼物)。
3. 收礼结束并确认返回帝江号大世界后，启动送礼阶段，再打开干员联络界面。
4. 每轮只选择一名送礼对象（点击后须经[选中态校验](#选中态校验)确认，实现见 `GiftOperatorContact.json`）：
    - **任意干员**：切换为信赖度升序，候选器以已有的 31 名干员头像识别当前列表中的目标，每轮返回一个尚未成功或排除的身份。
    - **只向信赖未满的干员送礼**：使用同一头像候选路径，在送礼界面读取信赖与每日上限，确认仍可提升信赖后才赠送；不再使用旧的多人选取节点按列表信赖数字筛选。
    - **指定干员**：`GiftOperatorSendCandidate.attach.templates` 仅包含该干员头像，候选器只选择该身份。成功送礼或排除后不会重复选择；指定目标耗尽时结束并报告未完成人数，不为填满人数而重复送礼。
5. 确认呼唤；若干员未到位，按[预设朝向与坐标移动兜底](#召唤干员后找不到对话按钮怎么办)（实现见 `GiftOperatorNavigation.json`）。
6. 等待干员出现，进入对话并打开送礼界面。所有送礼对象选项均初次读取信赖与固定每日上限文字；信赖已为 `200%` 或今日已满时，排除该身份，不减少待送礼人数。
7. 可以赠送时，按每人礼物数选择礼物（选中后走[选中态校验](#选中态校验)）、确认赠送、跳过对话并离开。回到大世界后，再与同一干员对话并重开送礼界面，核对信赖确实增加，或每日状态从未满变为已达上限。
8. 只有 `observe_after` 核对并提交成功，才把 `remaining` 减一、将身份加入 `completed` 并从后续候选中排除。剩余人数不为零时继续下一位；扫描后没有可用目标时结束并报告未完成人数，包括人数大于可用目标数或指定单人已耗尽的情况。`finish` 汇总成功名单、排除名单与剩余人数；剩余人数大于零时返回未完成错误。

Go 的 `GiftOperatorCandidateRecognition` 按头像模板文件名生成固定身份标识，复用 `Operator/Operator.json` 的模板与五语言姓名；`attach.templates` 为空表示使用全部 31 名干员，指定列表则限制候选。`GiftOperatorSessionAction` 只维护待完成人数、候选身份、初次观察、成功和排除记录；Pipeline 负责选人、呼唤、对话、点击送礼、离场与重新打开同一干员的送礼界面。

成功依据使用稳定送礼界面的信赖百分比和固定文字「[今日赠礼可提升的信赖]已达上限，请明天再来吧」。选礼物时出现的提示「[今日赠礼可提升的信赖]已达上限，无法选择更多」只表示无法继续预选，不能单独作为送礼成功或人数扣减的证据。

信赖百分比显示为整数；即使实际有小幅增长，如果重新进入送礼界面后整数显示未变、每日上限状态也未变化，仍无法确认成功。此时停止任务，保留未完成计数，避免再次向同一干员赠礼。

两个阶段共用联络台导航与呼唤确认，但使用独立的选人、对话与名称白名单。`GiftOperatorCheckContact.next` 经 `[Anchor]GiftOperatorSelectPhase` 分派到收礼或送礼选人；`GiftOperatorConfirmSelect.next` 经 `[Anchor]GiftOperatorWaitChatPhase` 分派到 `GiftOperatorReceiveWaitChat` 或 `GiftOperatorWaitChat`。收礼只覆盖 `GiftOperatorReceiveName`，不会改变送礼选项配置的 `GiftOperatorName`。

## 路线二：只收礼物

对应选项「只收礼物」开启，通过禁用 `GiftOperatorSendMain`，在收礼阶段结束后直接结束任务。收礼流程与默认路线的第一阶段完全相同，不受赠送对象和赠送数量影响；不再提供「接受全部礼物」选项。

1. 同样先存放背包，再寻路至干员联络台。
2. 每轮先通过 `GiftOperatorReceiveListToTop` 反向滑动，并用 `ListCompleteRecognition` 确认联络列表回到顶部，避免保留的滚动位置漏掉前面的礼物；随后在列表中[识别带礼物图标的干员](#收礼模式如何正确选中目标干员)（实现见 `GiftOperatorReceiveFlow.json` 与 `Operator/Operator.json`），而非按信赖排序或指定干员选人。
3. 确认呼唤，进入对话，只点击「收下礼物」，不进入送礼分支。
4. 领取后跳过对话并离开，`GiftOperatorReceiveBackInWorld` 确认 `InDijiangWorld` 后，才通过 `GiftOperatorReceiveContinue` 开始下一轮收礼。
5. `GiftOperatorReceiveContinue.max_hit` 为 `4`，初次领取加后续四轮，共最多领取五份。计数只在收礼阶段入口清空，寻找下一份礼物时不会清空；该限制对应每日五份礼物，不是失败重试。
6. 若当天剩余不足五份，当前页无礼物时继续滑动，通过 `ListCompleteRecognition` 确认列表滑动前后不再变化，再关闭联络界面；确认返回大世界后结束收礼阶段。
7. 若背包已满，提示后结束任务。

## 特殊处理

### 选中态校验

本任务里「点一下」不等于「选中了」，联络台选干员与送礼界面选礼物共用同一套**三层串联**判断，避免空点或点偏：

```text
选中高亮颜色 → 高亮区域内的文字底色 → OCR 读取关键文字
```

#### 联络台选干员

实现位于 `GiftOperatorContact.json`。每次点击列表行后，用 `And` 同时满足：

1. **标签高亮颜色**：识别该行选中态的 HSV 色块（青绿色标签底）。
2. **序号文字底色**：以上一步命中区域为锚，再识别序号数字所在的文字底色。
3. **序号 OCR**：当前收礼和送礼每轮都只选一人，读取序号 `1` 确认目标已入列。

送礼候选器先记录单个干员身份，再重新识别该身份的头像并点击，避免沿用上一帧的点击框。所有送礼对象与收礼路线在点中目标后，校验序号 `1` 无误再点确认呼唤。

#### 送礼界面选礼物

实现位于 `GiftOperatorGiftFlow.json`，链路相同，仅锚点与 OCR 目标不同：

1. 先用颜色匹配在底部礼物栏定位可点击项并点击。
2. 再校验礼物格的选中高亮颜色 + 文字底色。
3. 最后 OCR 读取该格内的数量数字（`\d+`），确认礼物确实处于选中态，才继续点「确认赠送」。

维护时若选中态识别漂移，优先检查这三层的颜色阈值与 OCR 区域偏移，干员与礼物两处应对照排查。

### 收礼模式：如何正确选中目标干员

收礼不能靠干员名字 OCR 直接点列表，而是**先找礼物、再认头像、最后校验名字**，逻辑分布在 `GiftOperatorReceiveFlow.json` 与 `Operator/Operator.json`。

1. **第一步：定位「有礼物的行」**  
   在联络列表区域用 `Gift.png` / `Gift_2.png` 模板匹配礼物图标（`green_mask`）。命中后偏移到相邻的点击区域，选中该行干员。

2. **第二步：确认是哪位干员**  
   以礼物图标命中位置为锚点，在相邻区域二次匹配该干员头像（`Operators/<Name>.png`，同样 `green_mask`）。  
   匹配成功后，把收礼对话使用的独立名称 OCR 白名单 `GiftOperatorReceiveName` 改成这名干员的多语言名字，不修改送礼白名单 `GiftOperatorName`。
   这一步写在 `Operator/Operator.json`，每名干员各一条；新增干员时必须同步维护。

    > **举例**：联络列表里礼物行旁二次匹配到 `Operators/Gilberta.png`，白名单即收窄为「洁尔佩塔 / Gilberta / …」仅这名干员。呼唤后在大世界等待对话时，须同时看到对话图标且名称 OCR 命中该白名单才会点击；场上出现佩丽卡、伊冯等其他干员时，名称对不上，**不会误点**。

3. **第三步：确认选中态**  
   复用上方[选中态校验](#选中态校验)逻辑，确认列表行高亮且序号为 `1`，再点击黄色确认按钮呼唤。

4. **第四步：对话阶段二次校验**  
   干员到场后，同时识别「对话图标」和「干员名称 OCR」，两者都命中才发起交互。  
   这样即使列表里点中了礼物行，也能在对话前再挡一次「叫错人」的情况。

头像模板必须经过 `fill_gift_operator_green_box.py` 处理（绿色描边 + 右上角遮罩），否则 `green_mask` 匹配不稳定。Win32 与 ADB 各有一套图片，需分别处理。

每轮查找先由 `GiftOperatorReceiveListToTop` 回到顶部，再由 `GiftOperatorReceiveSelect` 向下扫描。当前屏找不到带礼物的干员时，`GiftOperatorReceiveSwipe` 滑动列表并等待画面稳定后继续扫描；`GiftOperatorReceiveListComplete` 使用 `ListCompleteRecognition` 比较滑动前后的列表，确认列表不再变化且当前页没有礼物后，结束收礼阶段。`GiftOperatorReceiveRoundEntry` 每轮同时重置 `GiftOperatorReceiveListTopComplete` 与 `GiftOperatorReceiveListComplete` 的 `attach.ready`，收礼份数保持不变。

### 召唤干员后：找不到对话按钮怎么办

呼唤确认后，任务会先等干员出现并尝试点击对话入口。若此时画面上还看不到可交互的对话按钮，不会一直傻等，而是进入**站位修正兜底**，逻辑在 `GiftOperatorNavigation.json`。

修正顺序固定为三组预设，每组各尝试一次（任务开头会清零计数，避免上轮残留）：

| 次序 | 朝向 | 移动目标 |
| ---- | ------------ | -------------- |
| 1 | 正西（270°） | (186.6, 175.0) |
| 2 | 正北（0°） | (188.0, 175.3) |
| 3 | 正东（90°） | (188.6, 176.2) |

每组都是「先转向 → 再短距离移动 → 等待角色停稳」，然后重新尝试寻找对话按钮。

若三组都试过仍找不到，任务报错结束并提示「寻找干员识别失败」，同时保留截图供排查。常见原因是干员刷在了预设区域外，或呼唤后站位与模板 ROI 偏差过大。

另外两处同类重试，用于应对干员走过来导致点击偏移：

- 找到对话按钮但点完没进对话 → 原地再试一次点击。
- 进了对话但右侧动作按钮还没出来 → 跳过按钮也会自我重试一次，再等赠送 / 收礼按钮出现。

### 送礼阶段选人的差异（对比收礼）

送礼每轮以一个干员头像的固定身份为候选，不需要先找礼物图标；收礼仍先找礼物图标，再确认干员头像。所有送礼选项都由 `GiftOperatorSendCandidate` 选人，指定干员仅限制 `attach.templates`；已经成功或排除的身份不会再次入选。
