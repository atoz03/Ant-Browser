import { useEffect, useState } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { FolderOpen, Layers } from 'lucide-react'
import { Button, Card, ConfirmModal, FormItem, Input, Modal, Select, Textarea, toast } from '../../../shared/components'
import type { BrowserCore, BrowserProfileInput, BrowserProxy, BrowserGroup, ProxyLocationResolveResult } from '../types'
import { browserProxyResolveLocation, createBrowserProfile, fetchAllTags, fetchBrowserCores, fetchBrowserProfiles, fetchBrowserProxies, fetchBrowserSettings, fetchGroups, openUserDataDir, updateBrowserProfile, validateProxyConfig } from '../api'
import { FingerprintPanel } from '../components/FingerprintPanel'
import { applyLocaleToFingerprintArgs } from '../utils/fingerprintSerializer'
import { TagInput } from '../components/TagInput'
import { GroupSelector } from '../components/GroupSelector'
import { ProxyPickerModal } from '../components/ProxyPickerModal'

const fallbackLowLaunchArgs = ['--disable-sync', '--no-first-run']
const directProxyID = '__direct__'
type ProxySourceMode = 'pool' | 'local'

function normalizeLaunchArgs(args: string[]): string[] {
  return (args || []).map(item => item.trim()).filter(Boolean)
}

function resolveDefaultLaunchArgs(args: string[]): string[] {
  const normalized = normalizeLaunchArgs(args)
  return normalized.length > 0 ? normalized : fallbackLowLaunchArgs
}

function resolvePoolProxySelection(
  proxyId: string,
  proxyConfig: string,
  proxies: BrowserProxy[],
): { mode: ProxySourceMode; proxyId: string; proxyConfig: string } {
  const normalizedProxyId = proxyId.trim()
  if (normalizedProxyId) {
    const matchedByID = proxies.find((proxy) => proxy.proxyId.trim() === normalizedProxyId)
    if (matchedByID?.proxyId) {
      return { mode: 'pool', proxyId: matchedByID.proxyId, proxyConfig: '' }
    }
  }

  const rawProxyConfig = proxyConfig.trim()
  const normalizedConfig = rawProxyConfig.toLowerCase()
  if (normalizedConfig) {
    const matchedByConfig = proxies.find((proxy) => (proxy.proxyConfig || '').trim().toLowerCase() === normalizedConfig)
    if (matchedByConfig?.proxyId) {
      return { mode: 'pool', proxyId: matchedByConfig.proxyId, proxyConfig: '' }
    }
    return { mode: 'local', proxyId: '', proxyConfig: rawProxyConfig }
  }

  const directProxy = proxies.find((proxy) => proxy.proxyId === directProxyID)
  return { mode: 'pool', proxyId: directProxy?.proxyId || '', proxyConfig: '' }
}

export function BrowserEditPage() {
  const { id } = useParams()
  const navigate = useNavigate()
  const isCreate = id === 'new'
  const [formData, setFormData] = useState<BrowserProfileInput>({
    profileName: '',
    userDataDir: '',
    coreId: '',
    fingerprintArgs: [],
    proxyId: directProxyID,
    proxyConfig: '',
    launchArgs: [],
    tags: [],
    keywords: [],
    groupId: '',
  })
  const [cores, setCores] = useState<BrowserCore[]>([])
  const [proxies, setProxies] = useState<BrowserProxy[]>([])
  const [groups, setGroups] = useState<BrowserGroup[]>([])
  const [launchArgsText, setLaunchArgsText] = useState('')
  const [allTags, setAllTags] = useState<string[]>([])
  const [saving, setSaving] = useState(false)
  const [proxyPickerOpen, setProxyPickerOpen] = useState(false)
  const [proxyMode, setProxyMode] = useState<ProxySourceMode>('pool')
  const [isDirty, setIsDirty] = useState(false)
  const [leaveConfirm, setLeaveConfirm] = useState(false)
  const [saveError, setSaveError] = useState('')
  const [locationResolving, setLocationResolving] = useState(false)
  const [locationResult, setLocationResult] = useState<ProxyLocationResolveResult | null>(null)

  useEffect(() => {
    const loadData = async () => {
      const [coreList, proxyList, tagList, groupList, settings] = await Promise.all([
        fetchBrowserCores(),
        fetchBrowserProxies(),
        fetchAllTags(),
        fetchGroups(),
        fetchBrowserSettings(),
      ])
      const resolvedDefaultLaunchArgs = resolveDefaultLaunchArgs(settings.defaultLaunchArgs || [])
      setCores(coreList)
      setProxies(proxyList)
      setAllTags(tagList)
      setGroups(groupList)

      if (isCreate) {
        const resolved = resolvePoolProxySelection('', '', proxyList)
        setProxyMode('pool')
        setFormData((prev) => ({
          ...prev,
          proxyId: resolved.proxyId || directProxyID,
          proxyConfig: '',
          fingerprintArgs: settings.defaultFingerprintArgs || [],
        }))
        setLaunchArgsText(resolvedDefaultLaunchArgs.join('\n'))
        return
      }
      const list = await fetchBrowserProfiles()
      const current = list.find(item => item.profileId === id)
      if (!current) return
      const currentLaunchArgs = normalizeLaunchArgs(current.launchArgs)
      const normalizedCoreId = !current.coreId || current.coreId.toLowerCase() === 'default'
        ? ''
        : current.coreId
      const resolvedProxy = resolvePoolProxySelection(current.proxyId || '', current.proxyConfig || '', proxyList)
      setProxyMode(resolvedProxy.mode)
      setFormData({
        profileName: current.profileName,
        userDataDir: current.userDataDir,
        coreId: normalizedCoreId,
        fingerprintArgs: current.fingerprintArgs,
        proxyId: resolvedProxy.proxyId,
        proxyConfig: resolvedProxy.proxyConfig,
        launchArgs: currentLaunchArgs,
        tags: current.tags,
        keywords: current.keywords || [],
        groupId: current.groupId || '',
      })
      setLaunchArgsText(currentLaunchArgs.join('\n'))
    }
    loadData()
  }, [id, isCreate])

  const handleChange = (field: keyof BrowserProfileInput, value: string | string[]) => {
    setIsDirty(true)
    setFormData(prev => {
      if (field === 'proxyId') {
        return { ...prev, proxyId: typeof value === 'string' ? value : '' }
      }
      return { ...prev, [field]: value }
    })
  }

  const handleProxyModeChange = (mode: ProxySourceMode) => {
    setIsDirty(true)
    setProxyMode(mode)
    if (mode === 'pool') {
      setFormData((prev) => {
        if (prev.proxyId.trim()) {
          return prev
        }
        const directProxy = proxies.find((proxy) => proxy.proxyId === directProxyID)
        return {
          ...prev,
          proxyId: directProxy?.proxyId || '',
        }
      })
    }
  }

  const handleSave = async () => {
    const resolvedProxyId = proxyMode === 'pool' ? (formData.proxyId || '').trim() : ''
    const resolvedProxyConfig = proxyMode === 'local' ? (formData.proxyConfig || '').trim() : ''
    if (proxyMode === 'local' && !resolvedProxyConfig) {
      setSaveError('请输入本地代理地址')
      return
    }

    const payload: BrowserProfileInput = {
      ...formData,
      proxyId: resolvedProxyId,
      proxyConfig: resolvedProxyConfig,
      launchArgs: normalizeLaunchArgs(launchArgsText.split('\n')),
    }
    if (proxyMode === 'pool' && !resolvedProxyId) {
      payload.proxyId = directProxyID
      payload.proxyConfig = ''
    }

    setSaving(true)
    try {
      const validation = await validateProxyConfig(payload.proxyConfig, payload.proxyId)
      if (!validation.supported) {
        setSaveError(validation.errorMsg || '代理配置无效')
        return
      }
      if (isCreate) {
        await createBrowserProfile(payload)
        toast.success('配置已创建')
      } else if (id) {
        await updateBrowserProfile(id, payload)
        toast.success('配置已更新')
      }
      setIsDirty(false)
      navigate('/browser/list')
    } catch (error: any) {
      setSaveError(typeof error === 'string' ? error : error?.message || '保存失败')
    } finally {
      setSaving(false)
    }
  }

  const handleBack = () => {
    if (isDirty) { setLeaveConfirm(true) } else { navigate('/browser/list') }
  }

  const handleApplyProxyLocation = async () => {
    if (proxyMode !== 'pool' || !formData.proxyId || formData.proxyId === directProxyID) {
      toast.error('请选择代理池中的非直连节点')
      return
    }
    setLocationResolving(true)
    setLocationResult(null)
    try {
      const result = await browserProxyResolveLocation(formData.proxyId)
      setLocationResult(result)
      if (!result.ok || !result.lang || !result.timezone) {
        toast.error(result.error || '无法根据代理 IP 匹配定位')
        return
      }
      const nextArgs = applyLocaleToFingerprintArgs(formData.fingerprintArgs, result.lang, result.timezone)
      handleChange('fingerprintArgs', nextArgs)
      toast.success(`已设置 ${result.lang} / ${result.timezone}`)
    } catch (error: unknown) {
      toast.error((error as Error)?.message || '代理定位失败')
    } finally {
      setLocationResolving(false)
    }
  }

  const defaultCore = cores.find(c => c.isDefault)
  const selectedPoolProxy = proxies.find((proxy) => proxy.proxyId === formData.proxyId)

  const handleOpenUserDataDir = async () => {
    if (!formData.userDataDir.trim()) {
      toast.error('请先输入用户数据目录')
      return
    }
    try {
      await openUserDataDir(formData.userDataDir)
    } catch (error: unknown) {
      toast.error((error as Error)?.message || '打开目录失败')
    }
  }

  const handleProxyListUpdated = (nextProxies: BrowserProxy[]) => {
    setProxies(nextProxies)
  }

  const handleProxyDeleted = (deletedProxyId: string, nextProxies: BrowserProxy[]) => {
    setProxies(nextProxies)
    if (formData.proxyId !== deletedProxyId) {
      return
    }

    const fallbackProxy = nextProxies.find((proxy) => proxy.proxyId === directProxyID)
    if (fallbackProxy) {
      handleChange('proxyId', fallbackProxy.proxyId)
      return
    }

    handleChange('proxyId', '')
  }

  return (
    <div className="space-y-5 animate-fade-in">
      <div className="flex items-center justify-between">
        <div>
          <h1 className="text-xl font-semibold text-[var(--color-text-primary)]">{isCreate ? '新建配置' : '编辑配置'}</h1>
          <p className="text-sm text-[var(--color-text-muted)] mt-1">完善指纹与启动参数</p>
        </div>
        <div className="flex gap-2">
          <Button variant="secondary" size="sm" onClick={handleBack}>返回列表</Button>
          <Button size="sm" onClick={handleSave} loading={saving}>保存配置</Button>
        </div>
      </div>

      <Card title="基础信息" subtitle="实例与配置名称">
        <div className="grid grid-cols-1 md:grid-cols-2 gap-4">
          <FormItem label="配置名称" required>
            <Input value={formData.profileName} onChange={e => handleChange('profileName', e.target.value)} placeholder="请输入配置名称" />
          </FormItem>
          <FormItem label="用户数据目录（留空自动生成）">
            <div className="flex gap-2">
              <Input
                value={formData.userDataDir}
                onChange={e => handleChange('userDataDir', e.target.value)}
                placeholder="留空自动生成"
                className="flex-1"
              />
              <Button variant="secondary" size="sm" onClick={handleOpenUserDataDir} title="在文件管理器中打开">
                <FolderOpen className="w-4 h-4" />
              </Button>
            </div>
          </FormItem>
          <FormItem label="内核">
            <Select
              value={formData.coreId}
              onChange={e => handleChange('coreId', e.target.value)}
              options={
                cores.length > 0 ? [
                  { value: '', label: defaultCore ? `使用默认 (${defaultCore.coreName})` : '使用默认内核' },
                  ...cores.map(c => ({ value: c.coreId, label: c.coreName })),
                ] : [
                  { value: '', label: '暂无内核，请添加内核' }
                ]
              }
            />
          </FormItem>
          <FormItem label="标签">
            <TagInput
              value={formData.tags}
              onChange={tags => handleChange('tags', tags)}
              suggestions={allTags}
              placeholder="输入标签后按回车，支持从已有标签选择"
            />
          </FormItem>
          <FormItem label="分组">
            <GroupSelector
              groups={groups}
              value={formData.groupId || ''}
              onChange={groupId => handleChange('groupId', groupId)}
              placeholder="未分组"
              className="w-full"
            />
          </FormItem>
        </div>
      </Card>

      <Card title="代理配置" subtitle="支持代理池节点或本地代理地址">
        <div className="grid grid-cols-1 gap-4">
          <FormItem label="代理来源">
            <Select
              value={proxyMode}
              onChange={e => handleProxyModeChange(e.target.value as ProxySourceMode)}
              options={[
                { value: 'pool', label: '代理池' },
                { value: 'local', label: '本地代理' },
              ]}
            />
          </FormItem>
          {proxyMode === 'pool' ? (
            <FormItem label="代理地址选择">
              <div className="flex gap-2">
                <Select
                  value={formData.proxyId}
                  onChange={e => { handleChange('proxyId', e.target.value); setLocationResult(null) }}
                  options={
                    proxies.length > 0
                      ? proxies.map(p => ({ value: p.proxyId, label: p.proxyName || p.proxyId }))
                      : [{ value: '', label: '暂无代理，请先到代理池创建' }]
                  }
                  className="flex-1"
                />
                <Button variant="secondary" size="sm" onClick={() => setProxyPickerOpen(true)} title="按分组选择代理">
                  <Layers className="w-4 h-4" />
                </Button>
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={handleApplyProxyLocation}
                  loading={locationResolving}
                  disabled={!formData.proxyId || formData.proxyId === directProxyID}
                >
                  按代理匹配定位
                </Button>
              </div>
              {locationResult && (
                <div className="mt-2 text-xs text-[var(--color-text-muted)]">
                  {locationResult.ok
                    ? `出口 ${locationResult.ip || '-'} · ${[locationResult.country, locationResult.region, locationResult.city].filter(Boolean).join(' / ') || '-'} · ${locationResult.lang} · ${locationResult.timezone}`
                    : locationResult.error || '未匹配到定位'}
                </div>
              )}
            </FormItem>
          ) : (
            <FormItem label="本地代理地址" hint="支持 http://、https://、socks5://">
              <Input
                value={formData.proxyConfig}
                onChange={e => handleChange('proxyConfig', e.target.value)}
                placeholder="http://127.0.0.1:7890"
              />
            </FormItem>
          )}
        </div>
        <p className="text-xs text-[var(--color-text-muted)] mt-2">
          {proxyMode === 'pool'
            ? `当前使用代理池节点${selectedPoolProxy?.proxyName ? `：${selectedPoolProxy.proxyName}` : '。'}`
            : '本地代理不会进入代理池，只对当前实例保存生效。'}
        </p>
      </Card>

      <ProxyPickerModal
        open={proxyPickerOpen}
        currentProxyId={formData.proxyId}
        onSelect={proxy => { handleChange('proxyId', proxy.proxyId); setLocationResult(null) }}
        onProxyListUpdated={handleProxyListUpdated}
        onProxyDeleted={handleProxyDeleted}
        onClose={() => setProxyPickerOpen(false)}
      />

      <Card title="指纹配置" subtitle="配置浏览器指纹参数">
        <FingerprintPanel
          value={formData.fingerprintArgs}
          onChange={args => handleChange('fingerprintArgs', args)}
        />
      </Card>

      <Card title="启动参数" subtitle={isCreate ? '新建时默认填入轻量参数模板，直接改这里即可' : '每行一个参数'}>
        <div className="space-y-2">
          <Textarea
            value={launchArgsText}
            onChange={e => { setLaunchArgsText(e.target.value); setIsDirty(true) }}
            rows={6}
            placeholder="--disable-sync"
          />
          {isCreate && (
            <p className="text-xs text-[var(--color-text-muted)]">这里默认就是轻量参数模板；需要更复杂的参数，直接在此基础上修改。</p>
          )}
        </div>
      </Card>

      <ConfirmModal
        open={leaveConfirm}
        onClose={() => setLeaveConfirm(false)}
        onConfirm={() => navigate('/browser/list')}
        title="放弃未保存的更改？"
        content="当前页面有未保存的修改，离开后将丢失这些更改。"
        confirmText="放弃并离开"
        cancelText="继续编辑"
        danger
      />

      <Modal
        open={!!saveError}
        onClose={() => setSaveError('')}
        title="保存失败"
        width="420px"
        footer={<Button onClick={() => setSaveError('')}>知道了</Button>}
      >
        <div className="text-[var(--color-text-secondary)]">{saveError}</div>
      </Modal>
    </div>
  )
}
