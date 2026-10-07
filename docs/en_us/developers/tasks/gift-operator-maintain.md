# Development Manual - Gift Operator Maintenance Documentation

This document explains the files and independent receive and give stages of `GiftOperator`.
This documentation was last updated on October 7, 2026.

## File Paths

| Path | Purpose |
| ------------------------------------------------------------------- | --------------------------------------------- |
| `assets/interface.json` | Task mounting (`dijiang_ship` group) |
| `assets/tasks/GiftOperator.json` | Task entry and interface options |
| `assets/resource/pipeline/GiftOperator/GiftOperatorMain.json` | Entry, Di Jiang ship location |
| `assets/resource/pipeline/GiftOperator/GiftOperatorNavigation.json` | Pathfinding and contact point interaction |
| `assets/resource/pipeline/GiftOperator/GiftOperatorContact.json` | Contact interface operator selection |
| `assets/resource/pipeline/GiftOperator/GiftOperatorReceiveFlow.json` | Receive-stage selection, collection, and daily five-gift count |
| `assets/resource/pipeline/GiftOperator/GiftOperatorGiftFlow.json` | Gift giving during dialogue |
| `agent/go-service/giftoperator/` | Single-recipient recognition and remaining, completed, and excluded recipient records |
| `tests/GiftOperator/test_gift_ui_status.json` | Gift UI trust, daily-limit, and selection-toast recognition tests |
| `assets/resource/pipeline/GiftOperator/GiftOperatorBagFull.json` | Bag full handling |
| `assets/resource/pipeline/GiftOperator/Operator/Operator.json` | Receive-stage operator identification and name whitelist |
| `assets/resource/image/GiftOperator/` | Win32 recognition images |
| `assets/resource_adb/image/GiftOperator/` | ADB recognition images |
| `assets/resource_adb/pipeline/GiftOperator/` | ADB Pipeline mirror |
| `tools/gift_operator/fill_gift_operator_green_box.py` | Operator avatar green_mask formatting |
| `assets/locales/interface/*.json` | Task, option, and operator name text |

## Paths to Modify When Adding a New Operator

When adding a new operator, at least the following 7 locations need to be updated synchronously (`<Name>` is the operator identifier, consistent with the template filename and option case name):

| # | Path | Description |
| --- | -------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 1 | `assets/resource/image/GiftOperator/Operators/<Name>.png` | Win32 operator avatar template; must be processed with `tools/gift_operator/fill_gift_operator_green_box.py` before storage |
| 2 | `assets/resource_adb/image/GiftOperator/Operators/<Name>.png` | ADB operator avatar template; processed similarly |
| 3 | `assets/tasks/GiftOperator.json` → `SelectOperator` | Add a recipient case in the UI and configure `GiftOperatorSendCandidate.attach.templates` and the give-stage name whitelist `GiftOperatorName` |
| 4 | `assets/resource/pipeline/GiftOperator/Operator/Operator.json` | Recognize operator avatars in the receive stage and override the separate name whitelist `GiftOperatorReceiveName` |
| 5 | `assets/resource/pipeline/GiftOperator/GiftOperatorReceiveFlow.json` → `GiftOperatorSelectGiftOp.next` | Append `GiftOperatorSelect_<Name>` to the receive-stage selection node's `next` array; otherwise, the operator node will not be triggered |
| 6 | `assets/resource/pipeline/GiftOperator/GiftOperatorGiftFlow.json` → `GiftOperatorSendCandidate.attach.operators` | Append `GiftOperatorSelect_<Name>` so the give-stage candidate recognizer can reuse that operator's avatar and multilingual names |
| 7 | `assets/locales/interface/*.json` → `operator.<Name>` | Operator display name for each language |

## Route 1: Default (Receive, Then Give)

Corresponds to the "Receive Only" option being disabled. The task entry `GiftOperatorMain` runs `StashBackpackSubTask`, `GiftOperatorReceiveMain`, and `GiftOperatorSendMain` in order through `SubTask`. Giving starts only after collection finishes and uses the configured recipients and quantity.

The recipient option `SelectOperator` offers "Any Operator", specific operators, and "Only Give Gifts to Operators Below Max Trust" (`AnyNonMaxTrust`). "Gift Recipient Count" (`GiftOperatorCount`) is a separate positive-integer input, defaulting to `1`; "Gift Count" (`GiftCount`) is the number of gifts per operator. These settings affect only giving and do not change collection of the five daily gifts.

1. Store the bag before starting the task.
2. Run the separate receive stage to collect the five daily gifts one at a time. If some gifts were already collected that day, finish the stage after scanning and collecting the remaining gifts. See [Route 2](#route-2-receive-only) for the detailed flow.
3. Once collection finishes and the task confirms a return to the Di Jiang world, start the give stage and reopen the contact interface.
4. Select exactly one recipient per round (must pass [Selection State Verification](#selection-state-verification) after clicking; implementation in `GiftOperatorContact.json`):
    - **Any Operator**: Switch to trust ascending order. The candidate recognizer matches the existing 31 operator avatars against the current list and returns one identity that has not been completed or excluded.
    - **Only Give Gifts to Operators Below Max Trust**: Use the same avatar candidate path and read trust and the daily limit in the gift UI, giving only when trust can still increase. The old multiple-selection nodes no longer filter numeric trust values in the contact list.
    - **Specific Operator**: `GiftOperatorSendCandidate.attach.templates` contains only that operator's avatar. Once completed or excluded, that identity cannot be selected again. Exhausting the specified recipient ends the stage and reports the uncompleted count, without repeating gifts to meet the requested count.
5. Confirm the call; if the operator is not in position, use [Preset Orientation and Coordinate Movement Fallback](#after-calling-operator-what-to-do-if-dialogue-button-not-found) (implementation in `GiftOperatorNavigation.json`).
6. Wait for the operator to appear, enter dialogue, and open the gift UI. All recipient options first read trust and the fixed daily-limit text. If trust is already `200%` or the daily limit is already reached, exclude that identity without reducing the remaining recipient count.
7. When eligible, select the configured number of gifts per operator (passing [Selection State Verification](#selection-state-verification)), confirm giving, skip dialogue, and leave. After returning to the world, talk to the same operator and reopen the gift UI to verify that trust increased or the daily-limit state changed from available to reached.
8. Only a successful `observe_after` observation and commit decrements `remaining`, adds the identity to `completed`, and excludes it from future candidates. Continue with another recipient while the count is nonzero. Exhausting available targets ends the stage and reports the uncompleted count, including when the requested count exceeds the available recipients or a specified single recipient is exhausted. `finish` summarizes completed identities, excluded identities, and the remaining count; it returns an incomplete-result error if that count is greater than zero.

Go's `GiftOperatorCandidateRecognition` derives a canonical identity from the avatar template filename and reuses templates and multilingual names from `Operator/Operator.json`. An empty `attach.templates` means all 31 operators; a specified list restricts candidates. `GiftOperatorSessionAction` only maintains remaining recipients, the reserved identity, initial observations, and completed/excluded records. Pipeline handles selection, calling, dialogue, gift clicks, leaving, and reopening the same operator's gift UI.

Success evidence comes from the stable gift UI's trust percentage and fixed text "[今日赠礼可提升的信赖]已达上限，请明天再来吧". The selection toast "[今日赠礼可提升的信赖]已达上限，无法选择更多" only means that further gift preselection is unavailable; it cannot independently establish successful gifting or decrement the recipient count.

Trust is displayed as an integer percentage. Even if the actual value increased slightly, an unchanged displayed percentage and unchanged daily-limit state after reopening the gift UI cannot establish success. Stop the task and retain the uncompleted count in that case, avoiding another gift to the same operator.

Both stages share contact-point navigation and call confirmation, but use separate selection, dialogue, and name-whitelist chains. `GiftOperatorCheckContact.next` dispatches through `[Anchor]GiftOperatorSelectPhase` to receive-stage or give-stage selection; `GiftOperatorConfirmSelect.next` dispatches through `[Anchor]GiftOperatorWaitChatPhase` to `GiftOperatorReceiveWaitChat` or `GiftOperatorWaitChat`. Collection only overrides `GiftOperatorReceiveName`, preserving the give-stage `GiftOperatorName` configured by the recipient option.

## Route 2: Receive Only

Corresponds to the "Receive Only" option being enabled. It disables `GiftOperatorSendMain`, so the task ends after collection. Collection is identical to the first stage of the default route and is independent of the configured gift recipients and quantity. The "Accept All Gifts" option has been removed.

1. Similarly, store the bag first, then navigate to the operator contact point.
2. At the start of each round, `GiftOperatorReceiveListToTop` scrolls in reverse and uses `ListCompleteRecognition` to confirm that the contact list has returned to the top, preventing a saved scroll position from hiding earlier gifts. Then [identify operators with gift icons](#receive-mode-how-to-correctly-select-the-target-operator) (implementation in `GiftOperatorReceiveFlow.json` and `Operator/Operator.json`), rather than selecting by trust order or specific operator.
3. Confirm the call and enter dialogue. Only click "Accept Gift"; do not enter a giving branch.
4. After collection, skip dialogue and leave. `GiftOperatorReceiveBackInWorld` must confirm `InDijiangWorld` before `GiftOperatorReceiveContinue` starts another collection round.
5. `GiftOperatorReceiveContinue.max_hit` is `4`: the initial collection plus four more rounds yields at most five gifts. The count is cleared only at the receive-stage entry, and remains intact when looking for the next gift. This limit represents the five daily gifts rather than failed-operation retries.
6. If fewer than five gifts remain that day, continue scrolling whenever the current page contains no gifts. `ListCompleteRecognition` confirms that the list no longer changes after scrolling, then the task closes the contact interface. The receive stage ends after confirming a return to the world.
7. If the bag is full, prompt and end the task.

## Special Handling

### Selection State Verification

In this task, "clicking once" does not equal "selected". The contact point operator selection and gift interface selection share the same **three-layer serial judgment** to avoid empty or off-target clicks:

```text
Selection highlight color → Text background color within the highlighted area → OCR read key text
```

#### Contact Point Operator Selection

Implementation is in `GiftOperatorContact.json`. After clicking a list row, use `And` to simultaneously satisfy:

1. **Tag Highlight Color**: Identify the HSV color block of the selected state in that row (cyan-green label background).
2. **Sequence Number Text Background**: Use the hit area from the previous step as an anchor, then identify the text background color of the sequence number.
3. **Sequence Number OCR**: Both receive and give stages now select one operator per round; read sequence number `1` to confirm that the target is queued.

The give-stage candidate recognizer reserves one operator identity, then recognizes that identity's avatar again before clicking, avoiding a click box from the previous frame. Every give-stage selection and the receive route verify sequence number `1` after clicking the target, then confirm the call.

#### Gift Interface Gift Selection

Implementation is in `GiftOperatorGiftFlow.json`, same link chain, only anchor and OCR target differ:

1. First, use color matching to locate and click a clickable item in the bottom gift bar.
2. Then verify the selection highlight color + text background color of the gift cell.
3. Finally, OCR reads the quantity number (`\d+`) in that cell, confirming the gift is indeed in a selected state, then proceed to click "Confirm Gift".

When maintaining, if selection state recognition drifts, prioritize checking the color thresholds and OCR area offsets for these three layers; both operator and gift locations should be checked in parallel.

### Receive Mode: How to Correctly Select the Target Operator

Receive mode cannot rely on OCR of operator names to directly click the list. Instead, it **first finds the gift, then recognizes the avatar, and finally verifies the name**. Logic is distributed in `GiftOperatorReceiveFlow.json` and `Operator/Operator.json`.

1. **Step 1: Locate the "Row with a Gift"**  
   In the contact list area, use `Gift.png` / `Gift_2.png` template matching to find the gift icon (`green_mask`). After a hit, offset to the adjacent click area and select that operator row.

2. **Step 2: Confirm Which Operator**  
   Use the gift icon hit position as an anchor, and in the adjacent area, perform secondary matching of that operator's avatar (`Operators/<Name>.png`, also `green_mask`).  
   After a successful match, set the separate receive-stage name OCR whitelist `GiftOperatorReceiveName` to this operator's multilingual name, preserving the give-stage whitelist `GiftOperatorName`.
   This step is written in `Operator/Operator.json`, one entry per operator; must be maintained synchronously when adding new operators.

    > **Example**: If `Operators/Gilberta.png` is secondarily matched next to a gift row in the contact list, the whitelist is narrowed to "Gilberta / Gilberta / …" for only that operator. After the call, when waiting for dialogue in the world, it must simultaneously see the dialogue icon and the name OCR hit that whitelist before clicking; if other operators like Pelica or Yvonne appear on the field, the names don't match, **no mis-clicks**.

3. **Step 3: Confirm Selection State**  
   Reuse the [Selection State Verification](#selection-state-verification) logic above, confirm the list row is highlighted and the sequence number is `1`, then click the yellow confirm button to call.

4. **Step 4: Secondary Verification During Dialogue Stage**  
   After the operator arrives, simultaneously recognize the "dialogue icon" and "operator name OCR"; interaction is initiated only if both hit.  
   This way, even if a gift row is clicked in the list, another check prevents "calling the wrong person" before dialogue.

Avatar templates must be processed by `fill_gift_operator_green_box.py` (green border + upper-right mask); otherwise, `green_mask` matching is unstable. Win32 and ADB each have their own set of images and must be processed separately.

Each search starts with `GiftOperatorReceiveListToTop` returning to the top, followed by `GiftOperatorReceiveSelect` scanning downward. If no operator with a gift is found on the current screen, `GiftOperatorReceiveSwipe` scrolls the list and waits for it to stabilize before scanning again. `GiftOperatorReceiveListComplete` uses `ListCompleteRecognition` to compare the list before and after scrolling; collection ends once the list no longer changes and the current page has no gifts. `GiftOperatorReceiveRoundEntry` resets `attach.ready` on both `GiftOperatorReceiveListTopComplete` and `GiftOperatorReceiveListComplete` for each new search, preserving the gift count.

### After Calling Operator: What to Do If Dialogue Button Not Found

After call confirmation, the task first waits for the operator to appear and attempts to click the dialogue entry. If the interactive dialogue button is still not visible on screen, it won't wait indefinitely but enters **Position Correction Fallback**, logic in `GiftOperatorNavigation.json`.

Correction sequence is fixed to three preset groups, each attempted once (the count is reset at the start of the task to avoid residuals from the previous round):

| Order | Orientation | Movement Target |
| ----- | ----------- | --------------- |
| 1 | West (270°) | (186.6, 175.0) |
| 2 | North (0°) | (188.0, 175.3) |
| 3 | East (90°) | (188.6, 176.2) |

Each group is "turn first → then move a short distance → wait for the character to stop", then re-attempt to find the dialogue button.

If all three groups are tried and still not found, the task ends with an error and prompts "Failed to find operator identification", while retaining a screenshot for troubleshooting. Common causes are the operator spawning outside the preset area, or the position after the call deviating too much from the template ROI.

Two other similar retries handle click offsets caused by the operator walking over:

- Dialogue button found but no dialogue entered after clicking → Retry click once in place.
- Dialogue entered but right-side action buttons not yet appeared → The skip button also self-retries once, then waits for give/receive buttons to appear.

### Give-Stage Operator Selection Differences (Compared to Receive)

Giving uses one canonical avatar identity as the candidate for each round, without first locating a gift icon; collection still finds a gift icon and then identifies the avatar. All give-stage selections use `GiftOperatorSendCandidate`; specific operators only restrict `attach.templates`. Completed and excluded identities cannot be selected again.
