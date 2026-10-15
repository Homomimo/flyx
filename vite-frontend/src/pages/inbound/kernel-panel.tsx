import type {
  KernelApiItem,
  KernelCapabilitiesApiData,
  KernelStateApiData,
  NodeApiItem,
} from "@/api/types";

import { useEffect, useState } from "react";
import toast from "react-hot-toast";

import { type GroupBy, GroupHeader, groupItems, useCollapsed } from "./group";
import { eligibleNodes } from "./inbound-form-modal";
import { KERNEL_STATUS } from "./presets";

import {
  controlKernel,
  getKernelLogs,
  getKernelVersions,
  installKernel,
} from "@/api";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import { Input } from "@/shadcn-bridge/heroui/input";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { Select, SelectItem } from "@/shadcn-bridge/heroui/select";

const modalBase =
  "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden";

export interface InstallRequest {
  nodeId: number;
  kernelType: string;
}

interface Props {
  caps: KernelCapabilitiesApiData | null;
  kernels: KernelApiItem[];
  nodes: NodeApiItem[];
  installRequest: InstallRequest | null;
  onInstallRequestHandled: () => void;
  onChanged: () => void;
  groupBy: GroupBy;
}

function shortVersion(v: string): string {
  const m = v.match(/\d+\.\d+\.\d+/);

  return m ? m[0] : v;
}

export function KernelPanel({
  caps,
  kernels,
  nodes,
  installRequest,
  onInstallRequestHandled,
  onChanged,
  groupBy,
}: Props) {
  const [isCollapsed, toggleCollapsed] = useCollapsed();
  const [releases, setReleases] = useState<{
    type: string;
    items: { version: string; prerelease: boolean }[];
    error: string;
  } | null>(null);
  const [customVersion, setCustomVersion] = useState(false);
  const [installOpen, setInstallOpen] = useState(false);
  const [installForm, setInstallForm] = useState({
    nodeId: "",
    kernelType: "xray",
    version: "latest",
  });
  const [installing, setInstalling] = useState(false);
  const [busy, setBusy] = useState<string | null>(null);
  const [logsFor, setLogsFor] = useState<KernelApiItem | null>(null);
  const [logs, setLogs] = useState<KernelStateApiData | null>(null);
  const [logsLoading, setLogsLoading] = useState(false);
  const [removeTarget, setRemoveTarget] = useState<KernelApiItem | null>(null);
  const [removeFailed, setRemoveFailed] = useState(false);

  useEffect(() => {
    if (!installRequest) return;
    setInstallForm({
      nodeId: String(installRequest.nodeId),
      kernelType: installRequest.kernelType,
      version: "latest",
    });
    setInstallOpen(true);
    onInstallRequestHandled();
  }, [installRequest, onInstallRequestHandled]);

  const kernelName = (type: string) =>
    caps?.kernels.find((k) => k.type === type)?.name ?? type;

  const openInstall = () => {
    setInstallForm({ nodeId: "", kernelType: "xray", version: "latest" });
    setInstallOpen(true);
  };

  const doInstall = async () => {
    if (!installForm.nodeId) {
      toast.error("请选择节点");

      return;
    }
    setInstalling(true);
    try {
      const res = await installKernel({
        nodeId: Number(installForm.nodeId),
        kernelType: installForm.kernelType,
        version: installForm.version.trim() || "latest",
      });

      if (res.code === 0) {
        toast.success("内核已安装并启动");
        setInstallOpen(false);
      } else {
        toast.error(res.msg || "安装失败", { duration: 8000 });
      }
      onChanged();
    } finally {
      setInstalling(false);
    }
  };

  const doControl = async (
    k: KernelApiItem,
    action: "start" | "stop" | "restart",
  ) => {
    setBusy(`${k.id}-${action}`);
    try {
      const res = await controlKernel(k.id, action);

      if (res.code === 0) {
        toast.success("操作成功");
      } else {
        toast.error(res.msg || "操作失败");
      }
      onChanged();
    } finally {
      setBusy(null);
    }
  };

  const openLogs = async (k: KernelApiItem) => {
    setLogsFor(k);
    setLogs(null);
    setLogsLoading(true);
    try {
      const res = await getKernelLogs(k.id);

      if (res.code === 0) {
        setLogs(res.data);
      } else {
        toast.error(res.msg || "获取日志失败");
      }
    } finally {
      setLogsLoading(false);
    }
  };

  const doRemove = async (force: boolean) => {
    if (!removeTarget) return;
    setBusy(`${removeTarget.id}-remove`);
    try {
      const res = await controlKernel(removeTarget.id, "remove", force);

      if (res.code === 0) {
        toast.success("已卸载");
        setRemoveTarget(null);
        onChanged();
      } else {
        setRemoveFailed(true);
        toast.error(res.msg || "卸载失败");
      }
    } finally {
      setBusy(null);
    }
  };

  const installNodeOptions = eligibleNodes(nodes);
  const versionListType =
    installOpen &&
    !caps?.kernels.find((k) => k.type === installForm.kernelType)?.versions
      ?.length
      ? installForm.kernelType
      : "";

  useEffect(() => {
    if (!versionListType || releases?.type === versionListType) return;
    setCustomVersion(false);
    getKernelVersions(versionListType)
      .then((res) =>
        setReleases({
          type: versionListType,
          items: res.code === 0 ? res.data || [] : [],
          error: res.code === 0 ? "" : res.msg || "获取版本列表失败",
        }),
      )
      .catch(() =>
        setReleases({
          type: versionListType,
          items: [],
          error: "获取版本列表失败",
        }),
      );
  }, [versionListType]);

  const releaseItems =
    releases?.type === installForm.kernelType ? releases.items : [];
  const pinnedVersions =
    caps?.kernels.find((k) => k.type === installForm.kernelType)?.versions ??
    [];

  const renderKernel = (k: KernelApiItem) => {
    const st = KERNEL_STATUS[k.status] ?? {
      label: k.status,
      color: "default" as const,
    };

    return (
      <Card key={k.id} className="shadow-sm">
        <CardBody className="space-y-2">
          <div className="flex items-start justify-between gap-2">
            <div className="min-w-0">
              <div className="font-semibold truncate">
                {k.nodeName || `节点 ${k.nodeId}`}
              </div>
              <div className="text-xs text-default-500">
                {kernelName(k.kernelType)}{" "}
                {k.installedVersion
                  ? shortVersion(k.installedVersion)
                  : k.version}
                {" · "}
                {k.inboundCount} 个入站
              </div>
            </div>
            <div className="flex gap-1 shrink-0">
              {!k.nodeOnline && (
                <Chip color="default" size="sm" variant="flat">
                  节点离线
                </Chip>
              )}
              <Chip color={st.color} size="sm" variant="flat">
                {st.label}
              </Chip>
            </div>
          </div>
          {k.lastError && (
            <p className="text-xs text-danger break-all line-clamp-3">
              {k.lastError}
            </p>
          )}
          {!k.synced && !k.lastError && (
            <p className="text-xs text-warning">
              配置尚未同步到节点，节点上线后会自动下发
            </p>
          )}
          <div className="flex flex-wrap gap-1.5 pt-1">
            {k.enabled ? (
              <Button
                isLoading={busy === `${k.id}-stop`}
                size="sm"
                variant="flat"
                onPress={() => doControl(k, "stop")}
              >
                停止
              </Button>
            ) : (
              <Button
                color="success"
                isLoading={busy === `${k.id}-start`}
                size="sm"
                variant="flat"
                onPress={() => doControl(k, "start")}
              >
                启动
              </Button>
            )}
            <Button
              isDisabled={!k.enabled}
              isLoading={busy === `${k.id}-restart`}
              size="sm"
              variant="flat"
              onPress={() => doControl(k, "restart")}
            >
              重启
            </Button>
            <Button size="sm" variant="flat" onPress={() => openLogs(k)}>
              日志
            </Button>
            <Button
              color="danger"
              size="sm"
              variant="flat"
              onPress={() => {
                setRemoveFailed(false);
                setRemoveTarget(k);
              }}
            >
              卸载
            </Button>
          </div>
        </CardBody>
      </Card>
    );
  };
  const groups = groupItems(
    kernels,
    (k) => (groupBy === "node" ? String(k.nodeId) : k.kernelType),
    (_key, k) =>
      groupBy === "node"
        ? k.nodeName || `节点 ${k.nodeId}`
        : kernelName(k.kernelType),
  );

  return (
    <div className="mb-6">
      <div className="flex items-center justify-between mb-3">
        <h3 className="text-base font-semibold">节点内核</h3>
        <Button color="primary" size="sm" variant="flat" onPress={openInstall}>
          安装内核
        </Button>
      </div>

      {kernels.length === 0 ? (
        <Card className="shadow-sm bg-default-50/50">
          <CardBody className="text-center py-8 text-sm text-default-500">
            还没有在任何节点上安装内核。先安装 Xray，再创建入站。
          </CardBody>
        </Card>
      ) : groups.length <= 1 ? (
        <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3">
          {kernels.map(renderKernel)}
        </div>
      ) : (
        <div className="space-y-1">
          {groups.map((g) => {
            const id = `kernel:${groupBy}:${g.key}`;
            const folded = isCollapsed(id);
            const bad = g.items.filter(
              (k) => k.status === "error" || !k.nodeOnline,
            ).length;

            return (
              <div key={g.key}>
                <GroupHeader
                  collapsed={folded}
                  count={g.items.length}
                  extra={
                    bad > 0 ? (
                      <Chip color="danger" size="sm" variant="flat">
                        {bad} 个异常
                      </Chip>
                    ) : undefined
                  }
                  title={g.title}
                  unit="个内核"
                  onToggle={() => toggleCollapsed(id)}
                />
                {!folded && (
                  <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-3 pb-2">
                    {g.items.map(renderKernel)}
                  </div>
                )}
              </div>
            );
          })}
        </div>
      )}

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={installOpen}
        placement="center"
        onOpenChange={setInstallOpen}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-xl font-bold">安装内核</h2>
                <p className="text-small text-default-500">
                  节点会从官方 Release 下载并校验
                  SHA256，首次安装可能需要一两分钟
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="space-y-3">
                  <Select
                    label="节点"
                    placeholder="选择节点"
                    selectedKeys={
                      installForm.nodeId ? [installForm.nodeId] : []
                    }
                    onSelectionChange={(keys) =>
                      setInstallForm((p) => ({
                        ...p,
                        nodeId: String(Array.from(keys)[0] ?? ""),
                      }))
                    }
                  >
                    {installNodeOptions.map((n) => (
                      <SelectItem
                        key={String(n.id)}
                        textValue={
                          n.status === 1 ? n.name : `${n.name}（离线）`
                        }
                      >
                        {n.status === 1 ? n.name : `${n.name}（离线）`}
                      </SelectItem>
                    ))}
                  </Select>
                  <Select
                    disabledKeys={(caps?.kernels ?? [])
                      .filter((k) => !k.implemented)
                      .map((k) => k.type)}
                    label="内核"
                    selectedKeys={[installForm.kernelType]}
                    onSelectionChange={(keys) =>
                      setInstallForm((p) => ({
                        ...p,
                        kernelType: String(Array.from(keys)[0] ?? "xray"),
                        version:
                          caps?.kernels.find(
                            (k) => k.type === String(Array.from(keys)[0]),
                          )?.versions?.[0] ?? "latest",
                      }))
                    }
                  >
                    {(caps?.kernels ?? []).map((k) => (
                      <SelectItem
                        key={k.type}
                        textValue={
                          k.implemented ? k.name : `${k.name}（即将支持）`
                        }
                      >
                        {k.implemented ? k.name : `${k.name}（即将支持）`}
                      </SelectItem>
                    ))}
                  </Select>
                  {pinnedVersions.length > 0 ? (
                    <Select
                      description="官方渠道不提供校验文件，仅开放已核对 SHA256 的版本"
                      label="版本"
                      selectedKeys={[
                        pinnedVersions.includes(installForm.version)
                          ? installForm.version
                          : pinnedVersions[0],
                      ]}
                      onSelectionChange={(keys) =>
                        setInstallForm((p) => ({
                          ...p,
                          version: String(Array.from(keys)[0] ?? ""),
                        }))
                      }
                    >
                      {pinnedVersions.map((v) => (
                        <SelectItem key={v} textValue={v}>
                          {v}
                        </SelectItem>
                      ))}
                    </Select>
                  ) : (
                    <>
                      {releaseItems.length > 0 && (
                        <Select
                          description="最新版出问题时可以选旧版本；预发布版不建议使用"
                          label="版本"
                          selectedKeys={[
                            customVersion
                              ? "custom"
                              : installForm.version === "latest" ||
                                  !installForm.version
                                ? "latest"
                                : installForm.version,
                          ]}
                          onSelectionChange={(keys) => {
                            const v = String(Array.from(keys)[0] ?? "latest");

                            setCustomVersion(v === "custom");
                            if (v !== "custom") {
                              setInstallForm((p) => ({ ...p, version: v }));
                            }
                          }}
                        >
                          {[
                            <SelectItem
                              key="latest"
                              textValue="最新版（latest）"
                            >
                              最新版（latest）
                            </SelectItem>,
                            ...releaseItems.map((r) => (
                              <SelectItem
                                key={r.version}
                                textValue={
                                  r.prerelease
                                    ? `${r.version}（预发布）`
                                    : r.version
                                }
                              >
                                {r.prerelease
                                  ? `${r.version}（预发布）`
                                  : r.version}
                              </SelectItem>
                            )),
                            <SelectItem key="custom" textValue="手动输入…">
                              手动输入…
                            </SelectItem>,
                          ]}
                        </Select>
                      )}
                      {(releaseItems.length === 0 || customVersion) && (
                        <Input
                          description={
                            releases?.error && releaseItems.length === 0
                              ? `${releases.error}，可手动填写版本号，latest 表示最新版`
                              : "latest 表示最新版，也可填写具体版本号"
                          }
                          label={
                            releaseItems.length > 0 ? "自定义版本" : "版本"
                          }
                          value={installForm.version}
                          variant="bordered"
                          onChange={(e) =>
                            setInstallForm((p) => ({
                              ...p,
                              version: e.target.value,
                            }))
                          }
                        />
                      )}
                    </>
                  )}
                </div>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                <Button
                  color="primary"
                  isLoading={installing}
                  onPress={doInstall}
                >
                  安装
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={Boolean(logsFor)}
        placement="center"
        scrollBehavior="inside"
        size="4xl"
        onOpenChange={(open) => !open && setLogsFor(null)}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-lg font-bold">
                  {logsFor?.nodeName} · {kernelName(logsFor?.kernelType ?? "")}{" "}
                  日志
                </h2>
                {logs && (
                  <p className="text-small text-default-500">
                    {logs.running ? `运行中 PID ${logs.pid}` : "未运行"}
                    {logs.restarts > 0
                      ? ` · 已自动重启 ${logs.restarts} 次`
                      : ""}
                  </p>
                )}
              </ModalHeader>
              <ModalBody>
                <pre className="text-xs whitespace-pre-wrap break-all bg-default-100 rounded-xl p-3 max-h-[60vh] overflow-auto">
                  {logsLoading
                    ? "加载中..."
                    : (logs?.logs ?? []).join("\n") || "暂无日志"}
                </pre>
              </ModalBody>
              <ModalFooter>
                <Button
                  variant="flat"
                  onPress={() => logsFor && openLogs(logsFor)}
                >
                  刷新
                </Button>
                <Button variant="light" onPress={onClose}>
                  关闭
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={Boolean(removeTarget)}
        placement="center"
        onOpenChange={(open) => !open && setRemoveTarget(null)}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader>
                <h2 className="text-lg font-bold text-danger">卸载内核</h2>
              </ModalHeader>
              <ModalBody>
                <p className="text-default-600">
                  确定从{" "}
                  <span className="font-semibold text-foreground">
                    {removeTarget?.nodeName}
                  </span>{" "}
                  卸载 {kernelName(removeTarget?.kernelType ?? "")} 吗？
                </p>
                <p className="text-small text-danger">
                  该内核下的 {removeTarget?.inboundCount ?? 0}{" "}
                  个入站会一起删除，且无法恢复。
                </p>
                {removeFailed && (
                  <p className="text-small text-warning">
                    节点卸载失败（可能离线）。可以强制删除面板记录，节点上线后会自动清理。
                  </p>
                )}
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={onClose}>
                  取消
                </Button>
                {removeFailed && (
                  <Button
                    color="warning"
                    isLoading={busy === `${removeTarget?.id}-remove`}
                    onPress={() => doRemove(true)}
                  >
                    强制删除
                  </Button>
                )}
                <Button
                  color="danger"
                  isLoading={busy === `${removeTarget?.id}-remove`}
                  onPress={() => doRemove(false)}
                >
                  确认卸载
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </div>
  );
}
