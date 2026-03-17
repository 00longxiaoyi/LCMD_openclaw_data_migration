package migrate

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

var (
	legacyBaseDir       = "/lzcapp/document/AppShareCenter"
	defaultPackage      = "iamxiaoe.lzcapp.openclaw"
	homeNodeRoot        = "/home/node"
	dataTargetRoot      = "/home/node/.openclaw"
	workspaceTargetRoot = "/home/node/clawd"
	appTargetRoot       = "/app"
	homeDataSymlinkPath = ""
	runShellCommand     = func(command string) (string, error) {
		cmd := exec.Command("bash", "-lc", command)
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	lookupUser      = user.Lookup
	userHomeDir     = os.UserHomeDir
	runMigration    = Run
	copyDirectory   = copyDirectoryWithProgress
	changeOwnership = func(path string) error {
		return runShellAction(fmt.Sprintf("sudo chown -R abc:abc %s", shQuote(path)))
	}
)

// DefaultPackageName 返回默认的 OpenClaw 包名。
func DefaultPackageName() string {
	return defaultPackage
}

// LegacyRoot 返回旧版 OpenClaw 在网盘中的根目录。
func LegacyRoot(packageName string) string {
	return filepath.Join(legacyBaseDir, packageName)
}

// DataSource 返回 openclaw-data 的旧版源目录。
func DataSource(packageName string) string {
	return filepath.Join(LegacyRoot(packageName), "openclaw-data")
}

// WorkspaceSource 返回 openclaw-workspace 的旧版源目录。
func WorkspaceSource(packageName string) string {
	return filepath.Join(LegacyRoot(packageName), "openclaw-workspace")
}

// AppSource 返回 openclaw-app 的旧版源目录。
func AppSource(packageName string) string {
	return filepath.Join(LegacyRoot(packageName), "openclaw-app")
}

// DirExists 判断给定路径是否存在且为目录。
func DirExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// RunInteractive 按交互方式收集用户确认信息并启动迁移。
func RunInteractive(input io.Reader, output io.Writer) error {
	if input == nil {
		return fmt.Errorf("输入流不能为空")
	}
	if output == nil {
		output = io.Discard
	}

	reader := bufio.NewReader(input)
	if _, err := fmt.Fprintln(output, "请先确认以下信息（直接回车默认为 Y）："); err != nil {
		return err
	}

	dataExposed, err := askYesNo(reader, output, "1. 是否已将 OpenClaw 的应用数据暴露到网盘中了？[Y/n] ")
	if err != nil {
		return err
	}
	if !dataExposed {
		return fmt.Errorf("请先将 OpenClaw 应用数据暴露到网盘后再执行迁移")
	}

	packageName, err := askPackageName(reader, output)
	if err != nil {
		return err
	}

	stoppedUsingLegacy, err := askYesNo(reader, output, "3. 迁移过程中请不要使用之前的 OpenClaw，避免生成新的数据没有同步。[Y/n] ")
	if err != nil {
		return err
	}
	if !stoppedUsingLegacy {
		return fmt.Errorf("请先停止使用旧版 OpenClaw，再执行迁移")
	}

	return runMigration(packageName, output)
}

// Run 执行实际迁移流程，并将数据、工作区和应用目录复制到新位置。
func Run(packageName string, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}

	packageName = strings.TrimSpace(packageName)
	if packageName == "" {
		packageName = defaultPackage
	}

	hasData := DirExists(DataSource(packageName))
	hasWorkspace := DirExists(WorkspaceSource(packageName))
	hasApp := DirExists(AppSource(packageName))
	if !hasData && !hasWorkspace && !hasApp {
		return fmt.Errorf("未找到可迁移目录，请检查 %s 下是否存在 openclaw-data、openclaw-workspace 或 openclaw-app", LegacyRoot(packageName))
	}

	resolvedHomeDataSymlinkPath := ""
	if hasData {
		var err error
		resolvedHomeDataSymlinkPath, err = resolveHomeDataSymlinkPath()
		if err != nil {
			return err
		}
	}

	steps := []step{
		newStep("开始准备 /home/node", "已准备 /home/node", "准备 /home/node", func() error {
			return ensureDir(homeNodeRoot)
		}),
	}

	if hasData {
		steps = append(steps,
			newStep("开始准备 /home/node/.openclaw", "已准备 /home/node/.openclaw", "准备 /home/node/.openclaw", func() error {
				return ensureDir(dataTargetRoot)
			}),
			newStep("开始复制 openclaw-data 到 /home/node/.openclaw", "已完成 openclaw-data 复制", "复制 openclaw-data 到 /home/node/.openclaw", func() error {
				return copyDirectory(DataSource(packageName), dataTargetRoot, "openclaw-data", output)
			}),
			newStep("开始修正 /home/node/.openclaw 权限", "已完成 /home/node/.openclaw 权限修正", "修正 /home/node/.openclaw 权限", func() error {
				return changeOwnership(dataTargetRoot)
			}),
			newStep("开始创建 ~/.openclaw 软链", "已创建 ~/.openclaw 软链", "创建 ~/.openclaw 软链", func() error {
				return recreateSymlink(resolvedHomeDataSymlinkPath, dataTargetRoot)
			}),
		)
	}

	if hasWorkspace {
		steps = append(steps,
			newStep("开始准备 /home/node/clawd", "已准备 /home/node/clawd", "准备 /home/node/clawd", func() error {
				return ensureDir(workspaceTargetRoot)
			}),
			newStep("开始复制 openclaw-workspace 到 /home/node/clawd", "已完成 openclaw-workspace 复制", "复制 openclaw-workspace 到 /home/node/clawd", func() error {
				return copyDirectory(WorkspaceSource(packageName), workspaceTargetRoot, "openclaw-workspace", output)
			}),
			newStep("开始修正 /home/node/clawd 权限", "已完成 /home/node/clawd 权限修正", "修正 /home/node/clawd 权限", func() error {
				return changeOwnership(workspaceTargetRoot)
			}),
		)
	}

	if hasApp {
		steps = append(steps,
			newStep("开始准备 /app", "已准备 /app", "准备 /app", func() error {
				return ensureDir(appTargetRoot)
			}),
			newStep("开始复制 openclaw-app 到 /app", "已完成 openclaw-app 复制", "复制 openclaw-app 到 /app", func() error {
				return copyDirectory(AppSource(packageName), appTargetRoot, "openclaw-app", output)
			}),
			newStep("开始修正 /app 权限", "已完成 /app 权限修正", "修正 /app 权限", func() error {
				return changeOwnership(appTargetRoot)
			}),
		)
	}

	for _, step := range steps {
		if _, err := fmt.Fprintln(output, step.startMessage); err != nil {
			return err
		}
		if err := step.run(); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(output, step.finishMessage); err != nil {
			return err
		}
	}

	return nil
}

type step struct {
	startMessage  string
	finishMessage string
	run           func() error
}

// newStep 创建一个带统一错误包装的迁移步骤。
func newStep(startMessage, finishMessage, action string, run func() error) step {
	return step{
		startMessage:  startMessage,
		finishMessage: finishMessage,
		run: func() error {
			if err := run(); err != nil {
				return fmt.Errorf("执行%s失败: %w", action, err)
			}
			return nil
		},
	}
}

// ensureDir 确保目标目录存在，不存在时递归创建。
func ensureDir(path string) error {
	return os.MkdirAll(path, 0o755)
}

// resolveHomeDataSymlinkPath 解析 ~/.openclaw 软链的创建位置，优先兼容测试覆盖与 sudo 场景。
func resolveHomeDataSymlinkPath() (string, error) {
	if strings.TrimSpace(homeDataSymlinkPath) != "" {
		return homeDataSymlinkPath, nil
	}

	if sudoUser := strings.TrimSpace(os.Getenv("SUDO_USER")); sudoUser != "" {
		resolvedUser, err := lookupUser(sudoUser)
		if err == nil {
			homeDir := strings.TrimSpace(resolvedUser.HomeDir)
			if homeDir == "" {
				return "", fmt.Errorf("用户 %s 的主目录为空", sudoUser)
			}
			return filepath.Join(homeDir, ".openclaw"), nil
		}
	}

	homeDir, err := userHomeDir()
	if err != nil {
		return "", err
	}
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		return "", fmt.Errorf("无法解析当前用户主目录")
	}
	return filepath.Join(homeDir, ".openclaw"), nil
}

// runShellAction 执行 shell 命令，并在失败时拼接命令输出。
func runShellAction(command string) error {
	output, err := runShellCommand(command)
	if err == nil {
		return nil
	}

	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return err
	}
	return fmt.Errorf("%w，输出：%s", err, trimmed)
}

// askYesNo 读取一个带默认 Y 的是/否问题。
func askYesNo(reader *bufio.Reader, output io.Writer, prompt string) (bool, error) {
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return false, err
	}

	answer, err := readTrimmedLine(reader)
	if err != nil {
		return false, err
	}

	switch strings.ToLower(answer) {
	case "", "y", "yes":
		return true, nil
	case "n", "no":
		return false, nil
	default:
		return false, fmt.Errorf("无效输入 %q，仅支持 Y/n", answer)
	}
}

// askPackageName 读取包名确认结果，并在需要时要求输入自定义包名。
func askPackageName(reader *bufio.Reader, output io.Writer) (string, error) {
	if _, err := fmt.Fprintf(output, "2. OpenClaw 的包名是否为 %s？[Y/n] ", defaultPackage); err != nil {
		return "", err
	}

	answer, err := readTrimmedLine(reader)
	if err != nil {
		return "", err
	}

	switch strings.ToLower(answer) {
	case "", "y", "yes":
		return defaultPackage, nil
	case "n", "no":
		if _, err := fmt.Fprint(output, "请输入完整包名: "); err != nil {
			return "", err
		}
		packageName, err := readTrimmedLine(reader)
		if err != nil {
			return "", err
		}
		packageName = strings.TrimSpace(packageName)
		if packageName == "" {
			return "", fmt.Errorf("包名不能为空")
		}
		return packageName, nil
	default:
		return strings.TrimSpace(answer), nil
	}
}

// readTrimmedLine 读取一行输入并去掉首尾空白字符。
func readTrimmedLine(reader *bufio.Reader) (string, error) {
	line, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	return strings.TrimSpace(line), nil
}

// copyDirectoryWithProgress 递归复制目录内容，并持续输出复制进度。
func copyDirectoryWithProgress(src, dst, label string, output io.Writer) error {
	if output == nil {
		output = io.Discard
	}

	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}
	if !srcInfo.IsDir() {
		return fmt.Errorf("%s 不是目录", src)
	}

	if err := ensureDir(dst); err != nil {
		return err
	}
	if err := clearDirContents(dst); err != nil {
		return err
	}

	totalBytes, err := directorySize(src)
	if err != nil {
		return err
	}

	progress := newCopyProgress(label, output, totalBytes)
	if err := progress.report(); err != nil {
		return err
	}

	err = filepath.Walk(src, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if relPath == "." {
			return nil
		}

		dstPath := filepath.Join(dst, relPath)
		mode := info.Mode()
		switch {
		case mode&os.ModeSymlink != 0:
			return copySymlink(path, dstPath)
		case info.IsDir():
			return os.MkdirAll(dstPath, mode.Perm())
		case mode.IsRegular():
			return copyFile(path, dstPath, mode.Perm(), progress)
		default:
			return fmt.Errorf("不支持的文件类型: %s", path)
		}
	})
	if err != nil {
		return err
	}

	return progress.finish()
}

// clearDirContents 清空目标目录下已有的所有内容。
func clearDirContents(path string) error {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if err := os.RemoveAll(filepath.Join(path, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// directorySize 统计目录下所有普通文件的总字节数。
func directorySize(path string) (int64, error) {
	var total int64
	err := filepath.Walk(path, func(current string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode().IsRegular() {
			total += info.Size()
		}
		return nil
	})
	return total, err
}

// copySymlink 复制一个符号链接到目标路径。
func copySymlink(src, dst string) error {
	target, err := os.Readlink(src)
	if err != nil {
		return err
	}
	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}
	return os.Symlink(target, dst)
}

// copyFile 按块复制单个文件，并更新整体复制进度。
func copyFile(src, dst string, mode os.FileMode, progress *copyProgress) error {
	if err := ensureDir(filepath.Dir(dst)); err != nil {
		return err
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
	if err != nil {
		return err
	}

	buf := make([]byte, 64*1024)
	for {
		n, readErr := in.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				return err
			}
			if err := progress.add(int64(n)); err != nil {
				out.Close()
				return err
			}
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			out.Close()
			return readErr
		}
	}

	if err := out.Close(); err != nil {
		return err
	}
	return nil
}

// recreateSymlink 重建一个指向目标路径的软链接。
func recreateSymlink(linkPath, targetPath string) error {
	if strings.TrimSpace(linkPath) == "" {
		return fmt.Errorf("软链路径不能为空")
	}
	if err := ensureDir(filepath.Dir(linkPath)); err != nil {
		return err
	}
	if err := os.RemoveAll(linkPath); err != nil {
		return err
	}
	return os.Symlink(targetPath, linkPath)
}

type copyProgress struct {
	label        string
	output       io.Writer
	totalBytes   int64
	copiedBytes  int64
	lastReportAt int64
}

// newCopyProgress 创建一个复制进度记录器。
func newCopyProgress(label string, output io.Writer, totalBytes int64) *copyProgress {
	return &copyProgress{
		label:      label,
		output:     output,
		totalBytes: totalBytes,
	}
}

// add 累计已复制字节数，并在达到阈值时输出进度。
func (p *copyProgress) add(n int64) error {
	p.copiedBytes += n
	if p.totalBytes == 0 {
		return nil
	}
	if p.copiedBytes == p.totalBytes || p.copiedBytes-p.lastReportAt >= 1<<20 {
		return p.report()
	}
	return nil
}

// finish 在复制结束时补发最后一次进度输出。
func (p *copyProgress) finish() error {
	if p.totalBytes == 0 {
		return p.report()
	}
	if p.copiedBytes < p.totalBytes {
		p.copiedBytes = p.totalBytes
	}
	if p.lastReportAt == p.copiedBytes {
		return nil
	}
	return p.report()
}

// report 将当前复制进度格式化后写入输出流。
func (p *copyProgress) report() error {
	p.lastReportAt = p.copiedBytes
	percent := 100
	if p.totalBytes > 0 {
		percent = int(p.copiedBytes * 100 / p.totalBytes)
		if percent > 100 {
			percent = 100
		}
	}
	_, err := fmt.Fprintf(p.output, "%s 复制进度 %s %d%% (%s/%s)\n", p.label, formatProgressBar(percent), percent, humanizeBytes(p.copiedBytes), humanizeBytes(p.totalBytes))
	return err
}

// formatProgressBar 根据百分比生成文本进度条。
func formatProgressBar(percent int) string {
	const width = 20
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	filled := percent * width / 100
	return "[" + strings.Repeat("=", filled) + strings.Repeat("-", width-filled) + "]"
}

// humanizeBytes 将字节数转换为易读的单位字符串。
func humanizeBytes(size int64) string {
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	value := float64(size)
	unit := units[0]
	for i := 0; i < len(units)-1 && value >= 1024; i++ {
		value /= 1024
		unit = units[i+1]
	}
	if unit == "B" {
		return fmt.Sprintf("%d B", size)
	}
	return fmt.Sprintf("%.1f %s", value, unit)
}

// shQuote 将字符串包装为安全的单引号 shell 参数。
func shQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}
