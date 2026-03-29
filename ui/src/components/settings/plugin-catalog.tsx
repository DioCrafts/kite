import { useRef, useState } from 'react'
import {
  IconPackage,
  IconPlayerPlay,
  IconPlayerStop,
  IconRefresh,
  IconTrash,
  IconUpload,
} from '@tabler/icons-react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { toast } from 'sonner'

import {
  AdminPluginInfo,
  installPlugin,
  reloadPlugin,
  setPluginEnabled,
  uninstallPlugin,
  useAdminPlugins,
} from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { DeleteConfirmationDialog } from '@/components/delete-confirmation-dialog'

function pluginStateBadge(state: AdminPluginInfo['state']) {
  switch (state) {
    case 'loaded':
      return <Badge variant="default">Loaded</Badge>
    case 'failed':
      return <Badge variant="destructive">Failed</Badge>
    case 'disabled':
      return <Badge variant="secondary">Disabled</Badge>
    case 'stopped':
      return <Badge variant="outline">Stopped</Badge>
    default:
      return <Badge variant="secondary">{state}</Badge>
  }
}

export function PluginCatalog() {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const { data: plugins = [], isLoading, isError, error } = useAdminPlugins()

  const fileInputRef = useRef<HTMLInputElement>(null)
  const [installOpen, setInstallOpen] = useState(false)
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [uninstallPlugin_, setUninstallPlugin] = useState<string | null>(null)

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['admin-plugins'] })

  const installMutation = useMutation({
    mutationFn: (file: File) => installPlugin(file),
    onSuccess: (info) => {
      toast.success(
        t('plugins.installed', 'Plugin {{name}} v{{version}} installed', {
          name: info.name,
          version: info.version,
        })
      )
      setInstallOpen(false)
      setSelectedFile(null)
      invalidate()
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })

  const uninstallMutation = useMutation({
    mutationFn: (name: string) => uninstallPlugin(name),
    onSuccess: (_data, name) => {
      toast.success(t('plugins.uninstalled', 'Plugin {{name}} uninstalled', { name }))
      invalidate()
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })

  const reloadMutation = useMutation({
    mutationFn: (name: string) => reloadPlugin(name),
    onSuccess: (_data, name) => {
      toast.success(t('plugins.reloaded', 'Plugin {{name}} reloaded', { name }))
      invalidate()
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })

  const enableMutation = useMutation({
    mutationFn: ({ name, enabled }: { name: string; enabled: boolean }) =>
      setPluginEnabled(name, enabled),
    onSuccess: (_data, { name, enabled }) => {
      toast.success(
        enabled
          ? t('plugins.enabled', 'Plugin {{name}} enabled', { name })
          : t('plugins.disabled', 'Plugin {{name}} disabled', { name })
      )
      invalidate()
    },
    onError: (err: Error) => {
      toast.error(err.message)
    },
  })

  if (isLoading) {
    return (
      <div className="flex items-center justify-center h-32 text-muted-foreground">
        Loading plugins…
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center justify-center h-32 text-destructive">
        {(error as Error)?.message ?? 'Failed to load plugins'}
      </div>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="text-lg font-semibold">
            {t('plugins.catalog.title', 'Plugin Catalog')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t(
              'plugins.catalog.description',
              'Install, configure, and manage Kite plugins'
            )}
          </p>
        </div>
        <Button onClick={() => setInstallOpen(true)} size="sm">
          <IconUpload size={16} className="mr-2" />
          {t('plugins.install', 'Install Plugin')}
        </Button>
      </div>

      {plugins.length === 0 ? (
        <Card>
          <CardContent className="flex flex-col items-center justify-center py-12 text-center text-muted-foreground">
            <IconPackage size={48} className="mb-4 opacity-30" />
            <p className="text-base font-medium mb-1">
              {t('plugins.empty.title', 'No plugins installed')}
            </p>
            <p className="text-sm">
              {t(
                'plugins.empty.description',
                'Upload a .tar.gz plugin archive to get started'
              )}
            </p>
          </CardContent>
        </Card>
      ) : (
        <div className="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          {plugins.map((plugin) => (
            <Card key={plugin.name}>
              <CardHeader className="pb-2">
                <div className="flex items-start justify-between gap-2">
                  <div className="flex-1 min-w-0">
                    <CardTitle className="text-base truncate">
                      {plugin.name}
                    </CardTitle>
                    <CardDescription className="text-xs">
                      v{plugin.version}
                      {plugin.author ? ` · ${plugin.author}` : ''}
                    </CardDescription>
                  </div>
                  {pluginStateBadge(plugin.state)}
                </div>
              </CardHeader>
              <CardContent className="space-y-3">
                {plugin.description && (
                  <p className="text-sm text-muted-foreground line-clamp-2">
                    {plugin.description}
                  </p>
                )}
                {plugin.error && (
                  <p className="text-xs text-destructive break-words">
                    {plugin.error}
                  </p>
                )}
                <div className="flex flex-wrap gap-2">
                  {plugin.state === 'loaded' ? (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        enableMutation.mutate({ name: plugin.name, enabled: false })
                      }
                      disabled={enableMutation.isPending}
                    >
                      <IconPlayerStop size={14} className="mr-1" />
                      {t('plugins.disable', 'Disable')}
                    </Button>
                  ) : (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() =>
                        enableMutation.mutate({ name: plugin.name, enabled: true })
                      }
                      disabled={enableMutation.isPending}
                    >
                      <IconPlayerPlay size={14} className="mr-1" />
                      {t('plugins.enable', 'Enable')}
                    </Button>
                  )}
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() => reloadMutation.mutate(plugin.name)}
                    disabled={reloadMutation.isPending}
                  >
                    <IconRefresh size={14} className="mr-1" />
                    {t('plugins.reload', 'Reload')}
                  </Button>
                  <Button
                    variant="destructive"
                    size="sm"
                    onClick={() => setUninstallPlugin(plugin.name)}
                  >
                    <IconTrash size={14} className="mr-1" />
                    {t('plugins.uninstall', 'Uninstall')}
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      )}

      {/* Install dialog */}
      <Dialog open={installOpen} onOpenChange={setInstallOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              {t('plugins.install_dialog.title', 'Install Plugin')}
            </DialogTitle>
            <DialogDescription>
              {t(
                'plugins.install_dialog.description',
                'Upload a .tar.gz plugin archive. The archive must contain a plugin directory with a manifest.yaml and the plugin binary.'
              )}
            </DialogDescription>
          </DialogHeader>

          <div className="space-y-3">
            <input
              ref={fileInputRef}
              type="file"
              accept=".tar.gz,application/gzip,application/x-gzip,application/x-tar"
              className="hidden"
              onChange={(e) => setSelectedFile(e.target.files?.[0] ?? null)}
            />
            <Button
              variant="outline"
              className="w-full"
              onClick={() => fileInputRef.current?.click()}
            >
              <IconUpload size={16} className="mr-2" />
              {selectedFile
                ? selectedFile.name
                : t('plugins.install_dialog.choose_file', 'Choose .tar.gz file')}
            </Button>
          </div>

          <DialogFooter>
            <Button
              variant="outline"
              onClick={() => {
                setInstallOpen(false)
                setSelectedFile(null)
              }}
            >
              {t('common.cancel', 'Cancel')}
            </Button>
            <Button
              disabled={!selectedFile || installMutation.isPending}
              onClick={() => {
                if (selectedFile) installMutation.mutate(selectedFile)
              }}
            >
              {installMutation.isPending
                ? t('common.installing', 'Installing…')
                : t('plugins.install', 'Install Plugin')}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Uninstall confirmation */}
      <DeleteConfirmationDialog
        open={uninstallPlugin_ !== null}
        onOpenChange={(open) => {
          if (!open) setUninstallPlugin(null)
        }}
        onConfirm={() => {
          if (uninstallPlugin_) {
            uninstallMutation.mutate(uninstallPlugin_)
            setUninstallPlugin(null)
          }
        }}
        resourceName={uninstallPlugin_ ?? ''}
        resourceType="plugin"
      />
    </div>
  )
}
