package arenaofgames

import maa "github.com/MaaXYZ/maa-framework-go/v4"

// Register 注册百趣演武手牌识别器。
func Register() {
	maa.AgentServerRegisterCustomRecognition(component, &Recognition{})
}
