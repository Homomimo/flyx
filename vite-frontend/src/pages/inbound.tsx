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
import { Card, CardBody, CardHeader } from "@/shadcn-bridge/heroui/card";
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
  const [searchKeyword, setSearchKeyword] = useLocalStorageState(
    "inbound-search-keyword",
    "",
  );
  const [isSearchVisible, setIsSearchVisible] = useState(false);

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

  const loadData = useCallback(async (initial = false) => {
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
      if (kernelRes.code === 0) setKernels(kernelRes.data || []);
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
  }, []);

  useEffect(() => {
    loadData(true);
    const timer = window.setInterval(() => loadData(false), 30000);

    return () => window.clearInterval(timer);
  }, [loadData]);

  const filtered = useMemo(() => {
    const kw = searchKeyword.trim().toLowerCase();

    if (!kw) return inbounds;

    return inbounds.filter((i) =>
      [i.name, i.nodeName, i.protocol, String(i.port), i.remark]
        .join(" ")
        .toLowerCase()
        .includes(kw),
    );
  }, [inbounds, searchKeyword]);

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
      setInstallRequest({ nodeId, kernelType });
    },
    [],
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

  if (loading) {
    return <PageLoadingState message="正在加载..." />;
  }

  return (
    <AnimatedPage className="px-3 lg:px-6 py-8">
      <KernelPanel
        caps={caps}
        installRequest={installRequest}
        kernels={kernels}
        nodes={nodes}
        onChanged={() => loadData(false)}
        onInstallRequestHandled={() => setInstallRequest(null)}
      />

      {inbounds.length > 0 && <SubscriptionCard />}

      <div className="flex flex-col sm:flex-row items-stretch sm:items-center justify-between mb-4 gap-3">
        <div className="flex-1 max-w-sm flex items-center gap-2">
          <h3 className="text-base font-semibold shrink-0">入站</h3>
          <SearchBar
            isVisible={isSearchVisible}
            placeholder="搜索名称、节点、协议、端口"
            value={searchKeyword}
            onChange={setSearchKeyword}
            onClose={() => setIsSearchVisible(false)}
            onOpen={() => setIsSearchVisible(true)}
          />
        </div>
        <Button
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
          <CardBody className="text-center py-16 flex flex-col items-center justify-center">
            <h3 className="text-lg font-medium mb-2">暂无入站</h3>
            <p className="text-default-500 text-sm max-w-sm">
              {kernels.length === 0
                ? "先在上方为落地节点安装内核，然后在这里创建 VLESS-Reality、SS2022 等入站。"
                : "点击“新建入站”，选择一个模板即可快速创建。"}
            </p>
          </CardBody>
        </Card>
      ) : (
        <StaggerList className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
          {filtered.map((item) => {
            const kst = KERNEL_STATUS[item.kernelStatus];
            const running = item.enabled && item.kernelStatus === "running";

            return (
              <StaggerItem key={item.id}>
                <Card className="shadow-sm h-full">
                  <CardHeader className="pb-2 md:pb-2">
                    <div className="flex justify-between items-start w-full gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <span
                            className={`shrink-0 w-2 h-2 rounded-full ${
                              running
                                ? "bg-success"
                                : item.enabled
                                  ? "bg-warning"
                                  : "bg-default-300"
                            }`}
                          />
                          <h3 className="font-semibold truncate">
                            {item.name}
                          </h3>
                        </div>
                        <p className="text-xs text-default-500 truncate mt-0.5">
                          {item.nodeName} ·{" "}
                          {item.listen === "::" ? "" : `${item.listen}:`}
                          {item.port}
                        </p>
                      </div>
                      <Switch
                        isDisabled={toggling === item.id}
                        isSelected={item.enabled}
                        size="sm"
                        onValueChange={() => handleToggle(item)}
                      />
                    </div>
                  </CardHeader>
                  <CardBody className="pt-0 space-y-3">
                    <div className="flex flex-wrap gap-1.5">
                      <Chip color="primary" size="sm" variant="flat">
                        {describeInbound(item)}
                      </Chip>
                      <Chip size="sm" variant="flat">
                        {KERNEL_LABELS[item.kernelType] ?? item.kernelType}
                      </Chip>
                      {item.relayCount > 0 && (
                        <Chip color="secondary" size="sm" variant="flat">
                          {item.relayCount} 个中转
                        </Chip>
                      )}
                      {item.relayOnly && (
                        <Chip size="sm" variant="flat">
                          仅中转
                        </Chip>
                      )}
                      {item.acceptProxyProtocol && (
                        <Chip size="sm" variant="flat">
                          真实 IP
                        </Chip>
                      )}
                      {item.relayWhitelist && (
                        <Chip color="warning" size="sm" variant="flat">
                          白名单
                        </Chip>
                      )}
                      {item.enabled &&
                        kst &&
                        item.kernelStatus !== "running" && (
                          <Chip color={kst.color} size="sm" variant="flat">
                            内核{kst.label}
                          </Chip>
                        )}
                    </div>
                    <div className="flex justify-between text-sm">
                      <span className="text-default-500">流量</span>
                      <span className="font-mono text-xs">
                        ↑ {formatTraffic(item.uploadBytes)} · ↓{" "}
                        {formatTraffic(item.downloadBytes)}
                      </span>
                    </div>
                    {item.remark && (
                      <p className="text-xs text-default-500 line-clamp-2">
                        {item.remark}
                      </p>
                    )}
                    <div className="grid grid-cols-4 gap-1.5">
                      <Button
                        className="w-full min-w-0 px-1"
                        color="primary"
                        size="sm"
                        variant="flat"
                        onPress={() => setSharing(item)}
                      >
                        分享
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        color="secondary"
                        size="sm"
                        variant="flat"
                        onPress={() => setRelaying(item)}
                      >
                        中转
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        size="sm"
                        variant="flat"
                        onPress={() => setDetails(item)}
                      >
                        参数
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        size="sm"
                        variant="flat"
                        onPress={() => openEdit(item)}
                      >
                        编辑
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        size="sm"
                        variant="flat"
                        onPress={() => setConfirm({ action: "reset", item })}
                      >
                        重置流量
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        size="sm"
                        variant="flat"
                        onPress={() =>
                          setConfirm({ action: "regenerate", item })
                        }
                      >
                        换密钥
                      </Button>
                      <Button
                        className="w-full min-w-0 px-1"
                        color="danger"
                        size="sm"
                        variant="flat"
                        onPress={() => setConfirm({ action: "delete", item })}
                      >
                        删除
                      </Button>
                    </div>
                  </CardBody>
                </Card>
              </StaggerItem>
            );
          })}
        </StaggerList>
      )}

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
