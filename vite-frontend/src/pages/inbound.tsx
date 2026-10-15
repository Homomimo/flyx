import type {
  InboundApiItem,
  KernelApiItem,
  KernelCapabilitiesApiData,
  NodeApiItem,
} from "@/api/types";

import { useCallback, useEffect, useMemo, useState } from "react";
import toast from "react-hot-toast";
import { Copy } from "lucide-react";

import {
  deleteInbound,
  getInboundList,
  getKernelCapabilities,
  getKernelList,
  getNodeList,
  regenerateInbound,
  resetInboundTraffic,
  toggleInbound,
} from "@/api";
import {
  AnimatedPage,
  StaggerItem,
  StaggerList,
} from "@/components/animated-page";
import { PageLoadingState } from "@/components/page-state";
import { SearchBar } from "@/components/search-bar";
import { useLocalStorageState } from "@/hooks/use-local-storage-state";
import {
  GroupHeader,
  GroupToggle,
  groupItems,
  useCollapsed,
  useGroupBy,
} from "@/pages/inbound/group";
import { ActionSheet } from "@/pages/inbound/action-sheet";
import { InboundFormModal } from "@/pages/inbound/inbound-form-modal";
import { type InstallRequest, KernelPanel } from "@/pages/inbound/kernel-panel";
import { RelayModal } from "@/pages/inbound/relay-modal";
import { ShareModal, SubscriptionCard, copyText } from "@/pages/inbound/share";
import {
  type InboundForm,
  KERNEL_LABELS,
  KERNEL_STATUS,
  describeInbound,
  emptyForm,
  formFromItem,
} from "@/pages/inbound/presets";
import { Button } from "@/shadcn-bridge/heroui/button";
import { Card, CardBody } from "@/shadcn-bridge/heroui/card";
import { Chip } from "@/shadcn-bridge/heroui/chip";
import {
  Modal,
  ModalBody,
  ModalContent,
  ModalFooter,
  ModalHeader,
} from "@/shadcn-bridge/heroui/modal";
import { Switch } from "@/shadcn-bridge/heroui/switch";
import { formatTraffic } from "@/utils/traffic";

const modalBase =
  "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden";

type ConfirmAction = "delete" | "regenerate" | "reset";

type PageTab = "inbounds" | "kernels" | "sub";

const TAB_KEY = "flvx_inbound_tab";

/** Remembers the selected tab; a convenience only, so storage errors are ignored. */
function usePageTab(): [PageTab, (t: PageTab) => void] {
  const [tab, setTabState] = useState<PageTab>(() => {
    try {
      const v = localStorage.getItem(TAB_KEY);

      return v === "kernels" || v === "sub" ? v : "inbounds";
    } catch {
      return "inbounds";
    }
  });
  const setTab = useCallback((t: PageTab) => {
    setTabState(t);
    try {
      localStorage.setItem(TAB_KEY, t);
    } catch {
      // Ignored.
    }
  }, []);

  return [tab, setTab];
}

const CONFIRM_TEXT: Record<
  ConfirmAction,
  { title: string; body: string; button: string; color: "danger" | "warning" }
> = {
  delete: {
    title: "删除入站",
    body: "删除后节点上的该服务会立即停止，且无法恢复。",
    button: "确认删除",
    color: "danger",
  },
  regenerate: {
    title: "重新生成密钥",
    body: "会重新生成 UUID / 密码 / Reality 密钥，已有客户端需要重新导入配置。",
    button: "重新生成",
    color: "warning",
  },
  reset: {
    title: "重置流量",
    body: "将该入站的上传和下载流量清零。",
    button: "重置",
    color: "warning",
  },
};

function detailRows(item: InboundApiItem): Array<[string, string]> {
  const host = item.shareHost || item.nodeServerIp;
  const rows: Array<[string, string]> = [
    ["地址", host],
    ["端口", String(item.port)],
    ["协议", describeInbound(item)],
  ];
  const s = item.settings;
  const t = item.transport;
  const sec = item.security;

  if (s.uuid) rows.push(["UUID", s.uuid]);
  if (s.username) rows.push(["用户名", s.username]);
  if (s.flow) rows.push(["Flow", s.flow]);
  if (s.method) rows.push(["加密方式", s.method]);
  if (s.password)
    rows.push([item.protocol === "snell" ? "PSK" : "密码", s.password]);
  if (s.snellVersion) rows.push(["Snell 版本", `v${s.snellVersion}`]);
  if (s.masquerade) rows.push(["伪装网站", s.masquerade]);
  if (s.portHopping) rows.push(["端口跳跃", `${s.portHopping}（UDP）`]);
  if (t.path) rows.push(["路径", t.path]);
  if (t.host) rows.push(["Host", t.host]);
  if (t.serviceName) rows.push(["serviceName", t.serviceName]);
  if (t.mode) rows.push(["XHTTP 模式", t.mode]);
  if (sec.sni) {
    rows.push([
      sec.type === "shadowtls" ? "ShadowTLS 伪装网站" : "SNI",
      sec.sni,
    ]);
  }
  if (sec.shadowTlsPassword) {
    rows.push(["ShadowTLS 密码", sec.shadowTlsPassword]);
    rows.push(["ShadowTLS 版本", "3"]);
  }
  if (sec.fingerprint) rows.push(["指纹 (fp)", sec.fingerprint]);
  if (sec.realityPublicKey)
    rows.push(["Reality 公钥 (pbk)", sec.realityPublicKey]);
  if (sec.realityShortIds?.length) {
    rows.push(["shortId (sid)", sec.realityShortIds[0]]);
  }
  if (sec.type === "tls" && sec.certMode === "self") {
    rows.push(["证书", "自签（客户端需允许不安全证书或固定证书）"]);
  }
  if (sec.type === "tls" && sec.certMode === "acme") {
    rows.push([
      "证书",
      `自动申请（${sec.acmeDns ? `${sec.acmeDns} DNS 验证` : "HTTP 验证"}）`,
    ]);
  }

  return rows;
}

export default function InboundPage() {
  const [loading, setLoading] = useState(true);
  const [caps, setCaps] = useState<KernelCapabilitiesApiData | null>(null);
  const [nodes, setNodes] = useState<NodeApiItem[]>([]);
  const [kernels, setKernels] = useState<KernelApiItem[]>([]);
  const [inbounds, setInbounds] = useState<InboundApiItem[]>([]);
  const [groupBy, setGroupBy] = useGroupBy();
  const [isCollapsed, toggleCollapsed] = useCollapsed();
  const [searchKeyword, setSearchKeyword] = useLocalStorageState(
    "inbound-search-keyword",
    "",
  );
  const [isSearchVisible, setIsSearchVisible] = useState(false);
  const [nodeFilter, setNodeFilter] = useState("");
  const [tab, setTab] = usePageTab();
  const [sheetFor, setSheetFor] = useState<number | null>(null);

  const [formOpen, setFormOpen] = useState(false);
  const [formInitial, setFormInitial] = useState<InboundForm>(() =>
    emptyForm(null),
  );
  const [installRequest, setInstallRequest] = useState<InstallRequest | null>(
    null,
  );
  const [details, setDetails] = useState<InboundApiItem | null>(null);
  const [sharing, setSharing] = useState<InboundApiItem | null>(null);
  const [relaying, setRelaying] = useState<InboundApiItem | null>(null);
  const [confirm, setConfirm] = useState<{
    action: ConfirmAction;
    item: InboundApiItem;
  } | null>(null);
  const [confirmLoading, setConfirmLoading] = useState(false);
  const [toggling, setToggling] = useState<number | null>(null);

  const loadData = useCallback(
    async (initial = false) => {
      if (initial) setLoading(true);
      try {
        const [capsRes, nodeRes, kernelRes, inboundRes] = await Promise.all([
          getKernelCapabilities(),
          getNodeList(),
          getKernelList(),
          getInboundList(),
        ]);

        if (capsRes.code === 0) setCaps(capsRes.data);
        if (nodeRes.code === 0) setNodes(nodeRes.data || []);
        if (kernelRes.code === 0) {
          setKernels(kernelRes.data || []);
          if (initial && (kernelRes.data || []).length === 0) {
            setTab("kernels");
          }
        }
        if (inboundRes.code === 0) {
          setInbounds(inboundRes.data || []);
        } else {
          toast.error(inboundRes.msg || "获取入站列表失败");
        }
      } catch {
        toast.error("加载数据失败");
      } finally {
        if (initial) setLoading(false);
      }
    },
    [setTab],
  );

  useEffect(() => {
    loadData(true);
    const timer = window.setInterval(() => loadData(false), 30000);

    return () => window.clearInterval(timer);
  }, [loadData]);

  const nodeOptions = useMemo(() => {
    const map = new Map<string, string>();

    for (const i of inbounds) {
      map.set(String(i.nodeId), i.nodeName || `节点 ${i.nodeId}`);
    }

    return [...map.entries()].sort((a, b) => a[1].localeCompare(b[1], "zh-CN"));
  }, [inbounds]);

  const filtered = useMemo(() => {
    const kw = searchKeyword.trim().toLowerCase();

    return inbounds.filter(
      (i) =>
        (!nodeFilter || String(i.nodeId) === nodeFilter) &&
        (!kw ||
          [i.name, i.nodeName, i.protocol, String(i.port), i.remark]
            .join(" ")
            .toLowerCase()
            .includes(kw)),
    );
  }, [inbounds, searchKeyword, nodeFilter]);

  const inboundGroups = groupItems(
    filtered,
    (i) => (groupBy === "node" ? String(i.nodeId) : i.kernelType),
    (_key, i) =>
      groupBy === "node"
        ? i.nodeName || `节点 ${i.nodeId}`
        : (KERNEL_LABELS[i.kernelType] ?? i.kernelType),
  );
  const showGroupToggle =
    new Set(inbounds.map((i) => i.nodeId)).size > 1 ||
    new Set(inbounds.map((i) => i.kernelType)).size > 1;

  const openCreate = () => {
    const firstKernelNode = kernels[0]?.nodeId;

    setFormInitial(
      emptyForm(caps, firstKernelNode ? String(firstKernelNode) : ""),
    );
    setFormOpen(true);
  };

  const openEdit = (item: InboundApiItem) => {
    setFormInitial(formFromItem(item));
    setFormOpen(true);
  };

  const handleInstallFromForm = useCallback(
    (nodeId: number, kernelType: string) => {
      setFormOpen(false);
      setTab("kernels");
      setInstallRequest({ nodeId, kernelType });
    },
    [setTab],
  );

  const handleToggle = async (item: InboundApiItem) => {
    setToggling(item.id);
    try {
      const res = await toggleInbound(item.id, !item.enabled);

      if (res.code !== 0) {
        toast.error(res.msg || "操作失败");
      } else if (res.msg && res.msg !== "操作成功") {
        toast(res.msg, { icon: "⚠️" });
      }
      await loadData(false);
    } finally {
      setToggling(null);
    }
  };

  const runConfirm = async () => {
    if (!confirm) return;
    setConfirmLoading(true);
    try {
      const { action, item } = confirm;
      const res =
        action === "delete"
          ? await deleteInbound(item.id)
          : action === "regenerate"
            ? await regenerateInbound(item.id)
            : await resetInboundTraffic(item.id);

      if (res.code !== 0) {
        toast.error(res.msg || "操作失败");

        return;
      }
      if (res.msg && res.msg !== "操作成功") {
        toast(res.msg, { icon: "⚠️" });
      } else {
        toast.success("操作成功");
      }
      setConfirm(null);
      await loadData(false);
    } finally {
      setConfirmLoading(false);
    }
  };

  const renderInbound = (item: InboundApiItem) => {
    const running = item.enabled && item.kernelStatus === "running";
    const kernelBad = item.enabled && item.kernelStatus !== "running";
    const where =
      groupBy === "node" && showGroupToggle
        ? (KERNEL_LABELS[item.kernelType] ?? item.kernelType)
        : item.nodeName;

    return (
      <div
        key={item.id}
        className="flex w-full cursor-pointer items-center gap-3 rounded-xl border border-divider bg-content1 px-3 py-2.5 text-left shadow-sm transition-colors hover:bg-default-100"
        role="button"
        tabIndex={0}
        onClick={() => setSheetFor(item.id)}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return;
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setSheetFor(item.id);
          }
        }}
      >
        <span
          className={`h-2 w-2 shrink-0 rounded-full ${
            running
              ? "bg-success"
              : item.enabled
                ? "bg-warning"
                : "bg-default-300"
          }`}
          title={
            running
              ? "运行中"
              : item.enabled
                ? `内核${KERNEL_STATUS[item.kernelStatus]?.label ?? "异常"}`
                : "已停用"
          }
        />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-1.5">
            <span className="truncate font-medium">{item.name}</span>
            {item.relayCount > 0 && (
              <Chip color="secondary" size="sm" variant="flat">
                {item.relayCount} 中转
              </Chip>
            )}
            {kernelBad && (
              <Chip color="warning" size="sm" variant="flat">
                内核{KERNEL_STATUS[item.kernelStatus]?.label ?? "异常"}
              </Chip>
            )}
          </div>
          <div className="truncate text-xs text-default-500">
            {describeInbound(item)} · {item.port}
            {where ? ` · ${where}` : ""}
            <span className="sm:hidden">
              {" "}
              · {formatTraffic(item.uploadBytes + item.downloadBytes)}
            </span>
          </div>
        </div>
        <div className="hidden shrink-0 text-right font-mono text-xs text-default-500 sm:block">
          <div>↑ {formatTraffic(item.uploadBytes)}</div>
          <div>↓ {formatTraffic(item.downloadBytes)}</div>
        </div>
        <div
          className="hidden shrink-0 gap-1 md:flex"
          role="presentation"
          onClick={(e) => e.stopPropagation()}
        >
          <Button
            color="primary"
            size="sm"
            variant="flat"
            onPress={() => setSharing(item)}
          >
            分享
          </Button>
          <Button
            color="secondary"
            size="sm"
            variant="flat"
            onPress={() => setRelaying(item)}
          >
            中转
          </Button>
        </div>
        <div
          className="shrink-0"
          role="presentation"
          onClick={(e) => e.stopPropagation()}
        >
          <Switch
            isDisabled={toggling === item.id}
            isSelected={item.enabled}
            size="sm"
            onValueChange={() => handleToggle(item)}
          />
        </div>
      </div>
    );
  };

  if (loading) {
    return <PageLoadingState message="正在加载..." />;
  }

  const sheetItem = inbounds.find((i) => i.id === sheetFor) ?? null;
  const kernelProblems = kernels.filter(
    (k) => k.status === "error" || !k.nodeOnline,
  ).length;
  const tabs: Array<{ key: PageTab; label: string; badge?: React.ReactNode }> =
    [
      { key: "inbounds", label: "入站", badge: inbounds.length || undefined },
      {
        key: "kernels",
        label: "内核",
        badge:
          kernelProblems > 0 ? (
            <span className="text-danger">{kernelProblems} 异常</span>
          ) : (
            kernels.length || undefined
          ),
      },
      { key: "sub", label: "订阅" },
    ];

  return (
    <AnimatedPage className="px-3 lg:px-6 py-6">
      <div className="mb-4 grid grid-cols-3 gap-1 rounded-xl bg-default-100 p-1 sm:max-w-md">
        {tabs.map((t) => (
          <button
            key={t.key}
            className={`rounded-lg px-3 py-1.5 text-sm transition-colors ${
              tab === t.key
                ? "bg-background font-medium shadow-sm"
                : "text-default-500"
            }`}
            type="button"
            onClick={() => setTab(t.key)}
          >
            {t.label}
            {t.badge !== undefined && (
              <span className="ml-1 text-xs text-default-400">{t.badge}</span>
            )}
          </button>
        ))}
      </div>

      {tab === "kernels" && (
        <KernelPanel
          caps={caps}
          installRequest={installRequest}
          kernels={kernels}
          nodes={nodes}
          onChanged={() => loadData(false)}
          onInstallRequestHandled={() => setInstallRequest(null)}
        />
      )}

      {tab === "sub" &&
        (inbounds.length > 0 ? (
          <SubscriptionCard />
        ) : (
          <Card className="shadow-sm bg-default-50/50">
            <CardBody className="text-center py-10 text-sm text-default-500">
              创建入站后，这里会生成一个订阅链接，包含全部入站和中转线路。
            </CardBody>
          </Card>
        ))}

      {tab === "inbounds" && (
        <>
          <div className="mb-3 flex flex-wrap items-center gap-2">
            <SearchBar
              isVisible={isSearchVisible}
              placeholder="搜索名称、节点、协议、端口"
              value={searchKeyword}
              onChange={setSearchKeyword}
              onClose={() => setIsSearchVisible(false)}
              onOpen={() => setIsSearchVisible(true)}
            />
            {nodeOptions.length > 1 && (
              <select
                aria-label="按节点筛选"
                className="h-8 max-w-[10rem] rounded-lg border border-divider bg-background px-2 text-sm"
                value={nodeFilter}
                onChange={(e) => setNodeFilter(e.target.value)}
              >
                <option value="">全部节点</option>
                {nodeOptions.map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </select>
            )}
            {showGroupToggle && (
              <GroupToggle value={groupBy} onChange={setGroupBy} />
            )}
            <Button
              className="ml-auto"
              color="primary"
              isDisabled={kernels.length === 0}
              size="sm"
              variant="flat"
              onPress={openCreate}
            >
              新建入站
            </Button>
          </div>

          {filtered.length === 0 ? (
            <Card className="shadow-sm bg-default-50/50">
              <CardBody className="text-center py-16 flex flex-col items-center justify-center gap-3">
                <h3 className="text-lg font-medium">
                  {inbounds.length === 0 ? "暂无入站" : "没有匹配的入站"}
                </h3>
                <p className="text-default-500 text-sm max-w-sm">
                  {kernels.length === 0
                    ? "先为落地节点安装内核，然后在这里创建 VLESS-Reality、SS2022 等入站。"
                    : inbounds.length === 0
                      ? "点击“新建入站”，选择一个模板即可快速创建。"
                      : "换个关键词或节点试试。"}
                </p>
                {kernels.length === 0 && (
                  <Button
                    color="primary"
                    size="sm"
                    variant="flat"
                    onPress={() => setTab("kernels")}
                  >
                    去安装内核
                  </Button>
                )}
              </CardBody>
            </Card>
          ) : !showGroupToggle ? (
            <StaggerList className="grid grid-cols-1 2xl:grid-cols-2 gap-2">
              {filtered.map((i) => (
                <StaggerItem key={i.id}>{renderInbound(i)}</StaggerItem>
              ))}
            </StaggerList>
          ) : (
            <div className="space-y-1">
              {inboundGroups.map((g) => {
                const id = `inbound:${groupBy}:${g.key}`;
                const folded = isCollapsed(id);
                const off = g.items.filter((i) => !i.enabled).length;

                return (
                  <div key={g.key}>
                    <GroupHeader
                      collapsed={folded}
                      count={g.items.length}
                      extra={
                        off > 0 ? (
                          <Chip size="sm" variant="flat">
                            {off} 个已停用
                          </Chip>
                        ) : undefined
                      }
                      title={g.title}
                      unit="个入站"
                      onToggle={() => toggleCollapsed(id)}
                    />
                    {!folded && (
                      <div className="grid grid-cols-1 2xl:grid-cols-2 gap-2 pb-3">
                        {g.items.map(renderInbound)}
                      </div>
                    )}
                  </div>
                );
              })}
            </div>
          )}
        </>
      )}

      <ActionSheet
        actions={
          sheetItem
            ? [
                {
                  key: "share",
                  label: "分享",
                  color: "primary" as const,
                  onPress: () => setSharing(sheetItem),
                },
                {
                  key: "relay",
                  label: "中转",
                  color: "secondary" as const,
                  onPress: () => setRelaying(sheetItem),
                },
                {
                  key: "details",
                  label: "参数",
                  onPress: () => setDetails(sheetItem),
                },
                {
                  key: "edit",
                  label: "编辑",
                  onPress: () => openEdit(sheetItem),
                },
                {
                  key: "reset",
                  label: "重置流量",
                  onPress: () =>
                    setConfirm({ action: "reset", item: sheetItem }),
                },
                {
                  key: "regenerate",
                  label: "换密钥",
                  onPress: () =>
                    setConfirm({ action: "regenerate", item: sheetItem }),
                },
                {
                  key: "delete",
                  label: "删除",
                  color: "danger" as const,
                  onPress: () =>
                    setConfirm({ action: "delete", item: sheetItem }),
                },
              ]
            : []
        }
        isOpen={Boolean(sheetItem)}
        subtitle={
          sheetItem &&
          `${sheetItem.nodeName} · ${
            sheetItem.listen === "::" || sheetItem.listen === "0.0.0.0"
              ? ""
              : `${sheetItem.listen}:`
          }${sheetItem.port}`
        }
        title={sheetItem?.name ?? ""}
        onClose={() => setSheetFor(null)}
      >
        {sheetItem && (
          <div className="space-y-2">
            <div className="flex flex-wrap gap-1.5">
              <Chip color="primary" size="sm" variant="flat">
                {describeInbound(sheetItem)}
              </Chip>
              <Chip size="sm" variant="flat">
                {KERNEL_LABELS[sheetItem.kernelType] ?? sheetItem.kernelType}
              </Chip>
              {sheetItem.relayOnly && (
                <Chip size="sm" variant="flat">
                  仅中转
                </Chip>
              )}
              {sheetItem.acceptProxyProtocol && (
                <Chip size="sm" variant="flat">
                  真实 IP
                </Chip>
              )}
              {sheetItem.relayWhitelist && (
                <Chip color="warning" size="sm" variant="flat">
                  白名单
                </Chip>
              )}
            </div>
            <div className="flex justify-between text-sm">
              <span className="text-default-500">流量</span>
              <span className="font-mono text-xs">
                ↑ {formatTraffic(sheetItem.uploadBytes)} · ↓{" "}
                {formatTraffic(sheetItem.downloadBytes)}
              </span>
            </div>
            {sheetItem.remark && (
              <p className="text-xs text-default-500">{sheetItem.remark}</p>
            )}
          </div>
        )}
      </ActionSheet>

      <ShareModal item={sharing} onClose={() => setSharing(null)} />
      <RelayModal
        item={relaying}
        onChanged={() => loadData(false)}
        onClose={() => setRelaying(null)}
      />

      <InboundFormModal
        caps={caps}
        initial={formInitial}
        isOpen={formOpen}
        kernels={kernels}
        nodes={nodes}
        onInstallKernel={handleInstallFromForm}
        onOpenChange={setFormOpen}
        onSaved={() => loadData(false)}
      />

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={Boolean(details)}
        placement="center"
        scrollBehavior="inside"
        size="2xl"
        onOpenChange={(open) => !open && setDetails(null)}
      >
        <ModalContent>
          {(onClose) => (
            <>
              <ModalHeader className="flex flex-col gap-1">
                <h2 className="text-lg font-bold">{details?.name}</h2>
                <p className="text-small text-default-500">
                  手动配置客户端时使用；一般直接用“分享”或订阅导入即可
                </p>
              </ModalHeader>
              <ModalBody>
                <div className="divide-y divide-divider">
                  {details &&
                    detailRows(details).map(([k, v]) => (
                      <div
                        key={k}
                        className="flex items-center justify-between gap-3 py-2"
                      >
                        <span className="text-sm text-default-500 shrink-0">
                          {k}
                        </span>
                        <div className="flex items-center gap-1 min-w-0">
                          <span className="text-sm font-mono break-all text-right">
                            {v}
                          </span>
                          <Button
                            isIconOnly
                            aria-label={`复制${k}`}
                            size="sm"
                            variant="light"
                            onPress={() => copyText(v)}
                          >
                            <Copy className="w-3.5 h-3.5" />
                          </Button>
                        </div>
                      </div>
                    ))}
                </div>
              </ModalBody>
              <ModalFooter>
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
        isOpen={Boolean(confirm)}
        placement="center"
        onOpenChange={(open) => !open && setConfirm(null)}
      >
        <ModalContent>
          {(onClose) =>
            confirm && (
              <>
                <ModalHeader>
                  <h2
                    className={`text-lg font-bold ${
                      confirm.action === "delete" ? "text-danger" : ""
                    }`}
                  >
                    {CONFIRM_TEXT[confirm.action].title}
                  </h2>
                </ModalHeader>
                <ModalBody>
                  <p className="text-default-600">
                    入站{" "}
                    <span className="font-semibold text-foreground">
                      {confirm.item.name}
                    </span>
                  </p>
                  <p className="text-small text-default-500">
                    {CONFIRM_TEXT[confirm.action].body}
                  </p>
                </ModalBody>
                <ModalFooter>
                  <Button variant="light" onPress={onClose}>
                    取消
                  </Button>
                  <Button
                    color={CONFIRM_TEXT[confirm.action].color}
                    isLoading={confirmLoading}
                    onPress={runConfirm}
                  >
                    {CONFIRM_TEXT[confirm.action].button}
                  </Button>
                </ModalFooter>
              </>
            )
          }
        </ModalContent>
      </Modal>
    </AnimatedPage>
  );
}
