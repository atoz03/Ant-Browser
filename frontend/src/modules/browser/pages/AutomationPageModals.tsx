import { Button, FormItem, Input, Modal, Select } from "../../../shared/components";
import { AUTOMATION_SCRIPT_TYPE_OPTIONS, type AutomationScriptType } from "../automationScripts";
import type { ImportMode, LocalImportKind } from "./AutomationPage.helpers";

interface CreateAutomationScriptModalProps {
  open: boolean;
  busyAction: "none" | "create" | "import";
  createName: string;
  createType: AutomationScriptType;
  onClose: () => void;
  onCreate: () => Promise<void>;
  onCreateNameChange: (value: string) => void;
  onCreateTypeChange: (value: AutomationScriptType) => void;
}

export function CreateAutomationScriptModal({
  open,
  busyAction,
  createName,
  createType,
  onClose,
  onCreate,
  onCreateNameChange,
  onCreateTypeChange,
}: CreateAutomationScriptModalProps) {
  return (
      <Modal
        open={open}
        onClose={onClose}
        title="新建脚本"
        width="460px"
        footer={
          <>
            <Button
              variant="secondary"
              onClick={onClose}
              disabled={busyAction !== "none"}
            >
              取消
            </Button>
            <Button
              onClick={() => void onCreate()}
              loading={busyAction === "create"}
            >
              创建
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <FormItem label="脚本名称">
            <Input
              value={createName}
              onChange={(event) => onCreateNameChange(event.target.value)}
              placeholder="例如：接管页面并截图"
            />
          </FormItem>
          <FormItem label="脚本类型">
            <Select
              value={createType}
              options={AUTOMATION_SCRIPT_TYPE_OPTIONS}
              onChange={(event) =>
                onCreateTypeChange(event.target.value as AutomationScriptType)
              }
            />
          </FormItem>
        </div>
      </Modal>
  );
}

interface ImportAutomationScriptModalProps {
  open: boolean;
  busyAction: "none" | "create" | "import";
  importMode: ImportMode;
  localImportKind: LocalImportKind;
  gitURL: string;
  gitRef: string;
  gitScriptPath: string;
  onClose: () => void;
  onImport: () => Promise<void>;
  onImportModeChange: (value: ImportMode) => void;
  onLocalImportKindChange: (value: LocalImportKind) => void;
  onGitURLChange: (value: string) => void;
  onGitRefChange: (value: string) => void;
  onGitScriptPathChange: (value: string) => void;
}

export function ImportAutomationScriptModal({
  open,
  busyAction,
  importMode,
  localImportKind,
  gitURL,
  gitRef,
  gitScriptPath,
  onClose,
  onImport,
  onImportModeChange,
  onLocalImportKindChange,
  onGitURLChange,
  onGitRefChange,
  onGitScriptPathChange,
}: ImportAutomationScriptModalProps) {
  return (
      <Modal
        open={open}
        onClose={onClose}
        title="导入脚本"
        width="720px"
        footer={
          <>
            <Button
              variant="secondary"
              onClick={onClose}
              disabled={busyAction !== "none"}
            >
              取消
            </Button>
            <Button
              onClick={() => void onImport()}
              loading={busyAction === "import"}
            >
              导入
            </Button>
          </>
        }
      >
        <div className="space-y-4">
          <div className="flex flex-wrap gap-2">
            {[
              { value: "local", label: "本地", disabled: false },
              { value: "git", label: "Git（进行中）", disabled: false },
              { value: "script-library", label: "脚本库（计划中）", disabled: true },
            ].map((item) => (
              <Button
                key={item.value}
                size="sm"
                variant={importMode === item.value ? "primary" : "secondary"}
                onClick={() => {
                  if (item.value === "script-library") {
                    return;
                  }
                  onImportModeChange(item.value as ImportMode);
                }}
                disabled={busyAction !== "none" || item.disabled}
              >
                {item.label}
              </Button>
            ))}
          </div>

          {importMode === "local" ? (
            <div className="space-y-3">
              <div className="flex flex-wrap gap-2">
                {[
                  { value: "file", label: "ZIP / 文件" },
                  { value: "directory", label: "文件夹" },
                ].map((item) => (
                  <Button
                    key={item.value}
                    size="sm"
                    variant={localImportKind === item.value ? "primary" : "secondary"}
                    onClick={() => onLocalImportKindChange(item.value as LocalImportKind)}
                    disabled={busyAction !== "none"}
                  >
                    {item.label}
                  </Button>
                ))}
              </div>
              <div className="rounded-xl border border-[var(--color-border-default)] bg-[var(--color-bg-secondary)] px-4 py-4 text-sm text-[var(--color-text-secondary)]">
                {localImportKind === "directory"
                  ? "点击导入后选择脚本文件夹，适合一整套本地脚本目录。"
                  : "点击导入后选择本地文件，支持标准 ZIP 脚本包、JSON 模板和单文件脚本。"}
              </div>
            </div>
          ) : null}

          {importMode === "git" ? (
            <div className="space-y-4">
              <div className="text-sm text-[var(--color-text-secondary)]">
                会先拉取仓库，再把脚本快照导入当前项目。可以只填一个脚本子目录，系统只扫描那个目录；不填时才会按仓库根目录解析。若入口是 `.ts/.cts/.mts`，需要设置页已开启 TypeScript 导入构建。
              </div>
              <FormItem label="仓库地址">
                <Input
                  value={gitURL}
                  onChange={(event) => onGitURLChange(event.target.value)}
                  placeholder="https://github.com/example/automation-scripts.git"
                />
              </FormItem>
              <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
                <FormItem label="分支 / Tag / Commit">
                  <Input
                    value={gitRef}
                    onChange={(event) => onGitRefChange(event.target.value)}
                    placeholder="main"
                  />
                </FormItem>
                <FormItem label="脚本路径">
                  <Input
                    value={gitScriptPath}
                    onChange={(event) => onGitScriptPathChange(event.target.value)}
                    placeholder="scripts/demo（留空=仓库根目录）"
                  />
                </FormItem>
              </div>
            </div>
          ) : null}
        </div>
      </Modal>
  );
}
