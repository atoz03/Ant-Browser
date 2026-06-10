import { useState } from 'react'
import { toast } from '../../../shared/components'
import type { BrowserProfile, BrowserProfileCopyOptions } from '../types'
import { BrowserCoreEditorModal, BrowserListHeader, BrowserListSettingsModal } from '../components/BrowserListLayout'
import { BatchToolbar } from '../components/BrowserListWidgets'
import { BrowserProfilesPanel } from '../components/BrowserProfilesPanel'
import { createBrowserProfileCopyOptions, isBrowserProfileCopyOptionsValid } from '../copyOptions'
import { buildBrowserProfileCopyName } from '../copyName'
import { resolveActionFeedback } from '../utils/actionErrors'
import { BrowserListDialogs } from './browserList/BrowserListDialogs'
import { useBrowserListDerived, useBrowserListViewState } from './browserList/useBrowserListViewState'
import { useBrowserListSettings } from './browserList/useBrowserListSettings'
import { useBrowserListData } from './browserList/useBrowserListData'
import { useBrowserProfileActions } from './browserList/useBrowserProfileActions'
import { warmupProfileProxyBeforeStart } from '../utils/proxyWarmup'
import {
  copyBrowserProfile,
  deleteBrowserProfile,
  startBrowserInstance,
  stopBrowserInstance,
} from '../api'

export function BrowserListPage() {
  const {
    viewMode,
    setViewMode,
    filters,
    setFilters,
    headerCollapsed,
    setHeaderCollapsed,
  } = useBrowserListViewState()

  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [batchLoading, setBatchLoading] = useState(false)

  // 代理不支持弹窗
  const [proxyErrorModal, setProxyErrorModal] = useState(false)
  const [proxyErrorMsg, setProxyErrorMsg] = useState('')
  const [opError, setOpError] = useState('')
  const [pendingStartId, setPendingStartId] = useState<string | null>(null)

  // 关键字弹窗
  const [kwModal, setKwModal] = useState<{ open: boolean; profile: BrowserProfile | null }>({ open: false, profile: null })

  const openKwModal = (profile: BrowserProfile) => setKwModal({ open: true, profile })
  const closeKwModal = () => setKwModal({ open: false, profile: null })

  // 复制弹窗
  const [copyModal, setCopyModal] = useState<{ open: boolean; profile: BrowserProfile | null }>({ open: false, profile: null })
  const [copyName, setCopyName] = useState('')
  const [copyOptions, setCopyOptions] = useState<BrowserProfileCopyOptions>(() => createBrowserProfileCopyOptions())
  const [copying, setCopying] = useState(false)

  const openCopyModal = (profile: BrowserProfile) => {
    setCopyName(buildBrowserProfileCopyName(profile.profileName))
    setCopyOptions(createBrowserProfileCopyOptions())
    setCopyModal({ open: true, profile })
  }
  const closeCopyModal = () => {
    setCopyModal({ open: false, profile: null })
    setCopyName('')
    setCopyOptions(createBrowserProfileCopyOptions())
  }
  const {
    settingsModalOpen,
    setSettingsModalOpen,
    settings,
    setSettings,
    fingerprintText,
    setFingerprintText,
    launchText,
    setLaunchText,
    startUrlsText,
    setStartUrlsText,
    savingSettings,
    cores,
    coreModalOpen,
    setCoreModalOpen,
    coreForm,
    setCoreForm,
    coreValidation,
    setCoreValidation,
    savingCore,
    expandModalOpen,
    setExpandModalOpen,
    redeeming,
    maxProfileLimit,
    loadCores,
    loadQuota,
    handleOpenSettings,
    handleSaveSettings,
    handleOpenCoreModal,
    handleValidateCorePath,
    handleSaveCore,
    handleDeleteCore,
    handleSetDefaultCore,
    handleOpenGithubStarGift,
  } = useBrowserListSettings()
  const {
    profiles,
    loading,
    proxies,
    groups,
    startingIds,
    stoppingIds,
    setStartingIds,
    setStoppingIds,
    updatePendingIds,
    updateProfilesState,
    mergeProfileState,
    loadProfiles,
  } = useBrowserListData({ loadQuota, loadCores })
  const {
    runningCount,
    allTags,
    filteredProfiles,
    resolveProfileCore,
    getProfileCoreLabel,
    isProfileStarting,
    isProfileStopping,
    isProfileBusy,
    getProfileStatus,
  } = useBrowserListDerived(profiles, cores, filters, startingIds, stoppingIds)
  const {
    handleStart,
    handleStartDirect,
    handleStop,
    handleRestart,
    handleDelete,
  } = useBrowserProfileActions({
    profiles,
    setProxyErrorModal,
    setProxyErrorMsg,
    setPendingStartId,
    setOpError,
    setStartingIds,
    setStoppingIds,
    updatePendingIds,
    mergeProfileState,
    loadProfiles,
  })
  // 批量操作
  const toggleSelect = (profileId: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      next.has(profileId) ? next.delete(profileId) : next.add(profileId)
      return next
    })
  }



  const handleSelectAll = () => {
    setSelectedIds(new Set(filteredProfiles.map(p => p.profileId)))
  }

  const handleDeselectAll = () => {
    setSelectedIds(new Set())
  }

  const handleBatchStart = async () => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return
    setBatchLoading(true)
    let success = 0, pending = 0, failed = 0
    const pendingMessages: string[] = []
    const failureMessages: string[] = []
    for (const id of ids) {
      const profile = profiles.find(p => p.profileId === id)
      if (!profile || profile.running) continue
      updatePendingIds(setStartingIds, id, true)
      try {
        await warmupProfileProxyBeforeStart(profile)
        const startedProfile = await startBrowserInstance(id)
        mergeProfileState(startedProfile)
        success++
      } catch (error: any) {
        const feedback = resolveActionFeedback(error, '实例启动失败')
        if (feedback.pendingAttach) {
          pending++
          pendingMessages.push(`${profile.profileName}：${feedback.message}`)
        } else {
          failed++
          failureMessages.push(`${profile.profileName}：${feedback.message}`)
        }
      } finally {
        updatePendingIds(setStartingIds, id, false)
      }
    }
    setBatchLoading(false)
    const summary = [`成功 ${success}`]
    if (pending > 0) summary.push(`待接管 ${pending}`)
    if (failed > 0) summary.push(`失败 ${failed}`)
    toast.success(`批量启动完成：${summary.join('，')}`)
    if (pendingMessages.length > 0) {
      const preview = pendingMessages.slice(0, 3)
      const more = pendingMessages.length > preview.length ? `\n另有 ${pendingMessages.length - preview.length} 个实例已打开窗口，仍在后台接管。` : ''
      toast.warning(`以下实例已打开窗口，仍在后台接管：\n${preview.join('\n')}${more}`)
    }
    if (failureMessages.length > 0) {
      const preview = failureMessages.slice(0, 3)
      const more = failureMessages.length > preview.length ? `\n另有 ${failureMessages.length - preview.length} 个实例启动失败，请逐个检查。` : ''
      toast.error(`以下实例启动失败：\n${preview.join('\n')}${more}`)
    }
    loadProfiles()
  }

  const handleBatchStop = async () => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return
    setBatchLoading(true)
    let success = 0, failed = 0
    for (const id of ids) {
      const profile = profiles.find(p => p.profileId === id)
      if (!profile || !profile.running) continue
      updatePendingIds(setStoppingIds, id, true)
      try {
        const stoppedProfile = await stopBrowserInstance(id)
        mergeProfileState(stoppedProfile)
        success++
      } catch {
        failed++
      } finally {
        updatePendingIds(setStoppingIds, id, false)
      }
    }
    setBatchLoading(false)
    toast.success(`批量停止完成：成功 ${success}${failed > 0 ? `，失败 ${failed}` : ''}`)
    loadProfiles()
  }

  const handleBatchDelete = async () => {
    const ids = Array.from(selectedIds)
    if (ids.length === 0) return
    if (!confirm(`确定删除选中的 ${ids.length} 个实例？`)) return
    setBatchLoading(true)
    for (const id of ids) {
      await deleteBrowserProfile(id)
    }
    setBatchLoading(false)
    setSelectedIds(new Set())
    toast.success(`已删除 ${ids.length} 个实例`)
    loadProfiles()
  }

  const handleCopy = async (profileId: string) => {
    if (!copyModal.profile) return
    setCopying(true)
    try {
      await copyBrowserProfile(profileId, copyName.trim(), copyOptions)
      toast.success('实例已复制')
      closeCopyModal()
      loadProfiles()
    } catch (error: any) {
      setOpError(typeof error === 'string' ? error : error?.message || '复制失败')
    } finally {
      setCopying(false)
    }
  }

  const copyConfirmDisabled =
    !copyName.trim() || !isBrowserProfileCopyOptionsValid(copyOptions)


  return (
    <div className="overflow-auto p-5 space-y-5 animate-fade-in h-full">
      <BrowserListHeader
        profileCount={profiles.length}
        filteredProfileCount={filteredProfiles.length}
        runningCount={runningCount}
        headerCollapsed={headerCollapsed}
        viewMode={viewMode}
        proxies={proxies}
        cores={cores}
        groups={groups}
        allTags={allTags}
        filters={filters}
        onFiltersChange={setFilters}
        onToggleHeaderCollapsed={() => setHeaderCollapsed((prev) => !prev)}
        onRefresh={() => { void loadProfiles() }}
        onOpenSettings={handleOpenSettings}
        onOpenExpandModal={() => {
          setExpandModalOpen(true)
          loadQuota()
        }}
        onViewModeChange={setViewMode}
      />

      {/* 批量操作工具栏 */}
      <BatchToolbar
        selectedCount={selectedIds.size}
        totalCount={filteredProfiles.length}
        onSelectAll={handleSelectAll}
        onDeselectAll={handleDeselectAll}
        onBatchStart={handleBatchStart}
        onBatchStop={handleBatchStop}
        onBatchDelete={handleBatchDelete}
        batchLoading={batchLoading}
      />

      <BrowserProfilesPanel
        loading={loading}
        viewMode={viewMode}
        profiles={filteredProfiles}
        proxies={proxies}
        selectedIds={selectedIds}
        resolveProfileCore={resolveProfileCore}
        getProfileCoreLabel={getProfileCoreLabel}
        getProfileStatus={getProfileStatus}
        isProfileStarting={isProfileStarting}
        isProfileStopping={isProfileStopping}
        isProfileBusy={isProfileBusy}
        onToggleSelect={toggleSelect}
        onSelectAll={handleSelectAll}
        onDeselectAll={handleDeselectAll}
        onRefreshProfiles={() => { void loadProfiles() }}
        onStart={(profileId) => { void handleStart(profileId) }}
        onStop={(profileId) => { void handleStop(profileId) }}
        onRestart={(profileId) => { void handleRestart(profileId) }}
        onOpenKeywords={openKwModal}
        onOpenCopy={openCopyModal}
        onDelete={(profileId) => { void handleDelete(profileId) }}
      />

      <BrowserListSettingsModal
        open={settingsModalOpen}
        settings={settings}
        fingerprintText={fingerprintText}
        launchText={launchText}
        startUrlsText={startUrlsText}
        savingSettings={savingSettings}
        cores={cores}
        onClose={() => setSettingsModalOpen(false)}
        onSave={handleSaveSettings}
        onSettingsChange={(patch) => setSettings((prev) => ({ ...prev, ...patch }))}
        onFingerprintTextChange={setFingerprintText}
        onLaunchTextChange={setLaunchText}
        onStartUrlsTextChange={setStartUrlsText}
        onAddCore={() => handleOpenCoreModal()}
        onEditCore={handleOpenCoreModal}
        onDeleteCore={handleDeleteCore}
        onSetDefaultCore={handleSetDefaultCore}
      />

      <BrowserCoreEditorModal
        open={coreModalOpen}
        coreForm={coreForm}
        coreValidation={coreValidation}
        savingCore={savingCore}
        onClose={() => setCoreModalOpen(false)}
        onSave={handleSaveCore}
        onValidate={handleValidateCorePath}
        onCoreFormChange={(patch) => {
          setCoreForm((prev) => ({ ...prev, ...patch }))
          if (Object.prototype.hasOwnProperty.call(patch, 'corePath')) {
            setCoreValidation(null)
          }
        }}
      />

      <BrowserListDialogs
        proxyErrorModal={proxyErrorModal}
        pendingStartId={pendingStartId}
        proxyErrorMsg={proxyErrorMsg}
        onCloseProxyError={() => {
          setProxyErrorModal(false)
          setPendingStartId(null)
        }}
        onStartDirect={() => {
          if (pendingStartId) {
            void handleStartDirect(pendingStartId)
          }
        }}
        startingDirect={pendingStartId ? startingIds.has(pendingStartId) : false}
        kwModal={kwModal}
        onCloseKeywords={closeKwModal}
        onKeywordsSaved={(keywords) => {
          updateProfilesState(prev => prev.map(p =>
            p.profileId === kwModal.profile!.profileId ? { ...p, keywords } : p
          ))
        }}
        expandModalOpen={expandModalOpen}
        onCloseExpand={() => setExpandModalOpen(false)}
        profilesCount={profiles.length}
        maxProfileLimit={maxProfileLimit}
        redeeming={redeeming}
        onOpenGithubStarGift={handleOpenGithubStarGift}
        copyModal={copyModal}
        copyName={copyName}
        copyOptions={copyOptions}
        onCopyNameChange={setCopyName}
        onCopyOptionsChange={setCopyOptions}
        onCloseCopy={closeCopyModal}
        onConfirmCopy={() => copyModal.profile && handleCopy(copyModal.profile.profileId)}
        copyConfirmDisabled={copyConfirmDisabled}
        copying={copying}
        opError={opError}
        onCloseOpError={() => setOpError('')}
      />
    </div>
  )
}
