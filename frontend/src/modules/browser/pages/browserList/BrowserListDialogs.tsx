import { Link } from 'react-router-dom'
import { ExternalLink, XCircle } from 'lucide-react'
import { Button, Modal } from '../../../../shared/components'
import { BrowserProfileCopyForm } from '../../components/BrowserProfileCopyForm'
import { KeywordsModal } from '../../components/KeywordsModal'
import type { BrowserProfile, BrowserProfileCopyOptions } from '../../types'

interface BrowserListDialogsProps {
  proxyErrorModal: boolean
  pendingStartId: string | null
  proxyErrorMsg: string
  onCloseProxyError: () => void
  onStartDirect: () => void
  startingDirect: boolean
  kwModal: { open: boolean; profile: BrowserProfile | null }
  onCloseKeywords: () => void
  onKeywordsSaved: (keywords: string[]) => void
  expandModalOpen: boolean
  onCloseExpand: () => void
  profilesCount: number
  maxProfileLimit: number
  redeeming: boolean
  onOpenGithubStarGift: () => void
  copyModal: { open: boolean; profile: BrowserProfile | null }
  copyName: string
  copyOptions: BrowserProfileCopyOptions
  onCopyNameChange: (value: string) => void
  onCopyOptionsChange: (value: BrowserProfileCopyOptions) => void
  onCloseCopy: () => void
  onConfirmCopy: () => void
  copyConfirmDisabled: boolean
  copying: boolean
  opError: string
  onCloseOpError: () => void
}

export function BrowserListDialogs({
  proxyErrorModal,
  pendingStartId,
  proxyErrorMsg,
  onCloseProxyError,
  onStartDirect,
  startingDirect,
  kwModal,
  onCloseKeywords,
  onKeywordsSaved,
  expandModalOpen,
  onCloseExpand,
  profilesCount,
  maxProfileLimit,
  redeeming,
  onOpenGithubStarGift,
  copyModal,
  copyName,
  copyOptions,
  onCopyNameChange,
  onCopyOptionsChange,
  onCloseCopy,
  onConfirmCopy,
  copyConfirmDisabled,
  copying,
  opError,
  onCloseOpError,
}: BrowserListDialogsProps) {
  return (
    <>
      <Modal
        open={proxyErrorModal}
        onClose={onCloseProxyError}
        title="代理链路不可用"
        width="420px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseProxyError} disabled={startingDirect}>取消</Button>
            {pendingStartId && (
              <Button variant="secondary" onClick={onStartDirect} loading={startingDirect}>
                直连启动
              </Button>
            )}
            {pendingStartId && (
              <Link to={`/browser/edit/${pendingStartId}`}>
                <Button onClick={onCloseProxyError} disabled={startingDirect}>去修改代理</Button>
              </Link>
            )}
          </>
        }
      >
        <div className="space-y-3">
          <div className="flex items-start gap-3 p-3 rounded-lg bg-[var(--color-bg-secondary)]">
            <XCircle className="w-5 h-5 text-red-500 mt-0.5 shrink-0" />
            <p className="text-sm text-[var(--color-text-primary)]">{proxyErrorMsg}</p>
          </div>
          <p className="text-sm text-[var(--color-text-muted)]">请前往编辑页面重新选择可用链路；如果是订阅导入，先刷新订阅并确认该节点仍存在。</p>
        </div>
      </Modal>

      {kwModal.profile && (
        <KeywordsModal
          open={kwModal.open}
          profileId={kwModal.profile.profileId}
          profileName={kwModal.profile.profileName}
          initialKeywords={kwModal.profile.keywords || []}
          onClose={onCloseKeywords}
          onSaved={onKeywordsSaved}
        />
      )}

      <Modal
        open={expandModalOpen}
        onClose={onCloseExpand}
        title="实例扩容系统"
        width="480px"
        footer={<Button variant="secondary" onClick={onCloseExpand}>关闭</Button>}
      >
        <div className="space-y-4">
          <div className="rounded-xl border border-[var(--color-accent)]/35 bg-[var(--color-accent)]/10 p-4 shadow-sm">
            <div>
              <p className="text-xs text-[var(--color-text-muted)]">当前容量</p>
              <p className="text-xs text-[var(--color-text-secondary)] mt-2">每个配置消耗 1 个实例额度</p>
            </div>
            <div className="text-right">
              <span className={`text-3xl font-semibold ${profilesCount >= maxProfileLimit ? 'text-red-500' : 'text-[var(--color-accent)]'}`}>
                {profilesCount}
              </span>
              <span className="text-sm text-[var(--color-text-muted)] ml-1">/ {maxProfileLimit}</span>
            </div>
          </div>

          <div className="rounded-xl border border-[var(--color-border-default)] bg-[var(--color-bg-subtle)] p-4">
            <div className="flex items-center justify-between gap-4">
              <p className="text-sm font-medium text-[var(--color-text-primary)]">点亮 GitHub Star 后领取 50 个永久额度</p>
              <Button
                size="lg"
                onClick={onOpenGithubStarGift}
                loading={redeeming}
                className="shrink-0 shadow-sm"
                title="打开 GitHub 并领取赠送"
              >
                <ExternalLink className="w-4 h-4" />
                Star 扩容 +50
              </Button>
            </div>
          </div>
        </div>
      </Modal>

      <Modal
        open={copyModal.open}
        onClose={onCloseCopy}
        title="复制实例"
        width="720px"
        footer={
          <>
            <Button variant="secondary" onClick={onCloseCopy}>取消</Button>
            <Button onClick={onConfirmCopy} loading={copying} disabled={copyConfirmDisabled}>确认复制</Button>
          </>
        }
      >
        <BrowserProfileCopyForm
          sourceName={copyModal.profile?.profileName}
          copyName={copyName}
          copyOptions={copyOptions}
          onCopyNameChange={onCopyNameChange}
          onCopyOptionsChange={onCopyOptionsChange}
          autoFocusName
        />
      </Modal>

      <Modal
        open={!!opError}
        onClose={onCloseOpError}
        title="操作失败"
        width="420px"
        footer={<Button onClick={onCloseOpError}>知道了</Button>}
      >
        <div className="text-[var(--color-text-secondary)] whitespace-pre-line">{opError}</div>
      </Modal>
    </>
  )
}
