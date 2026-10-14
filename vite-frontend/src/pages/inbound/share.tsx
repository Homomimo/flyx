import type { InboundApiItem, InboundShareEntry } from "@/api/types";

import axios from "axios";
import QRCode from "qrcode";
import { useEffect, useState } from "react";
import toast from "react-hot-toast";
import { Copy, QrCode, RefreshCw } from "lucide-react";

import { getInboundShare, getSubInfo } from "@/api";
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

const modalBase =
  "!w-[calc(100%-32px)] !mx-auto sm:!w-full rounded-2xl overflow-hidden";

export async function copyText(text: string, label = "已复制") {
  try {
    await navigator.clipboard.writeText(text);
    toast.success(label);
  } catch {
    toast.error("复制失败，请手动复制");
  }
}

/** The panel origin the browser talks to, so links work behind a proxy. */
function panelOrigin(): string {
  const base = axios.defaults.baseURL || "";

  try {
    return new URL(base, window.location.origin).origin;
  } catch {
    return window.location.origin;
  }
}

export function QrImage({ text, size = 220 }: { text: string; size?: number }) {
  const [src, setSrc] = useState("");

  useEffect(() => {
    let alive = true;

    QRCode.toDataURL(text, {
      margin: 1,
      width: size,
      errorCorrectionLevel: "M",
    })
      .then((url) => alive && setSrc(url))
      .catch(() => alive && setSrc(""));

    return () => {
      alive = false;
    };
  }, [text, size]);

  if (!src) {
    return (
      <div
        className="flex items-center justify-center text-xs text-default-500 bg-default-100 rounded-xl"
        style={{ width: size, height: size }}
      >
        内容过长，无法生成二维码
      </div>
    );
  }

  return (
    <img
      alt="二维码"
      className="rounded-xl bg-white p-2"
      height={size}
      src={src}
      width={size}
    />
  );
}

export function ShareModal({
  item,
  onClose,
}: {
  item: InboundApiItem | null;
  onClose: () => void;
}) {
  const [entries, setEntries] = useState<InboundShareEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [qrFor, setQrFor] = useState<string | null>(null);

  useEffect(() => {
    if (!item) return;
    setEntries([]);
    setQrFor(null);
    setLoading(true);
    getInboundShare(item.id)
      .then((res) => {
        if (res.code === 0) {
          setEntries(res.data || []);
        } else {
          toast.error(res.msg || "获取分享链接失败");
        }
      })
      .finally(() => setLoading(false));
  }, [item]);

  return (
    <Modal
      backdrop="blur"
      classNames={{ base: modalBase }}
      isOpen={Boolean(item)}
      placement="center"
      scrollBehavior="inside"
      size="2xl"
      onOpenChange={(open) => !open && onClose()}
    >
      <ModalContent>
        {(close) => (
          <>
            <ModalHeader className="flex flex-col gap-1">
              <h2 className="text-lg font-bold">分享 · {item?.name}</h2>
              <p className="text-small text-default-500">
                复制链接或扫码导入 v2rayN、Shadowrocket、NekoBox 等客户端
              </p>
            </ModalHeader>
            <ModalBody>
              {loading && <p className="text-sm text-default-500">加载中...</p>}
              {!loading && entries.length === 0 && (
                <p className="text-sm text-default-500">
                  没有可分享的地址：入站仅监听本机或设置了仅中转，但还没有对接中转。
                </p>
              )}
              <div className="space-y-3">
                {entries.map((e) => (
                  <div
                    key={`${e.kind}-${e.host}-${e.port}`}
                    className="rounded-xl border border-divider p-3 space-y-2"
                  >
                    <div className="flex items-center justify-between gap-2">
                      <div className="min-w-0">
                        <div className="flex items-center gap-2">
                          <Chip
                            color={e.kind === "relay" ? "secondary" : "primary"}
                            size="sm"
                            variant="flat"
                          >
                            {e.kind === "relay" ? "中转" : "直连"}
                          </Chip>
                          <span className="font-medium truncate">{e.name}</span>
                        </div>
                        <p className="text-xs text-default-500 font-mono mt-1">
                          {e.host}:{e.port}
                        </p>
                      </div>
                      <div className="flex gap-1 shrink-0">
                        <Button
                          isIconOnly
                          aria-label="二维码"
                          size="sm"
                          variant="flat"
                          onPress={() =>
                            setQrFor(qrFor === e.uri ? null : e.uri)
                          }
                        >
                          <QrCode className="w-4 h-4" />
                        </Button>
                        <Button
                          color="primary"
                          size="sm"
                          startContent={<Copy className="w-3.5 h-3.5" />}
                          variant="flat"
                          onPress={() => copyText(e.uri, "链接已复制")}
                        >
                          复制
                        </Button>
                      </div>
                    </div>
                    <p className="text-xs font-mono break-all text-default-600 line-clamp-2">
                      {e.uri}
                    </p>
                    {qrFor === e.uri && (
                      <div className="flex justify-center pt-1">
                        <QrImage text={e.uri} />
                      </div>
                    )}
                  </div>
                ))}
              </div>
            </ModalBody>
            <ModalFooter>
              <Button variant="light" onPress={close}>
                关闭
              </Button>
            </ModalFooter>
          </>
        )}
      </ModalContent>
    </Modal>
  );
}

const SUB_FORMATS: { key: string; label: string; hint: string }[] = [
  { key: "", label: "自动识别", hint: "按客户端自动返回对应格式" },
  {
    key: "clash",
    label: "Clash / Mihomo",
    hint: "Clash Verge、Stash、FlClash",
  },
  { key: "singbox", label: "sing-box", hint: "SFA / SFI / SFM" },
  { key: "base64", label: "通用", hint: "v2rayN、Shadowrocket、NekoBox" },
  { key: "surge", label: "Surge", hint: "不含 VLESS / Reality" },
];

export function SubscriptionCard() {
  const [path, setPath] = useState("");
  const [showQr, setShowQr] = useState(false);
  const [resetOpen, setResetOpen] = useState(false);
  const [resetting, setResetting] = useState(false);

  const load = (reset = false) =>
    getSubInfo(reset).then((res) => {
      if (res.code === 0) {
        setPath(res.data.path);

        return true;
      }
      toast.error(res.msg || "获取订阅失败");

      return false;
    });

  useEffect(() => {
    load();
  }, []);

  const url = (format: string) =>
    path ? `${panelOrigin()}${path}${format ? `?format=${format}` : ""}` : "";

  const doReset = async () => {
    setResetting(true);
    try {
      if (await load(true)) {
        toast.success("订阅地址已重置，旧地址立即失效");
        setResetOpen(false);
      }
    } finally {
      setResetting(false);
    }
  };

  return (
    <Card className="shadow-sm mb-6">
      <CardBody className="space-y-3">
        <div className="flex items-center justify-between gap-2">
          <div>
            <h3 className="text-base font-semibold">订阅</h3>
            <p className="text-xs text-default-500">
              包含全部已启用入站及其中转，客户端定期更新即可同步
            </p>
          </div>
          <div className="flex gap-1">
            <Button
              isIconOnly
              aria-label="订阅二维码"
              size="sm"
              variant="flat"
              onPress={() => setShowQr((v) => !v)}
            >
              <QrCode className="w-4 h-4" />
            </Button>
            <Button
              isIconOnly
              aria-label="重置订阅地址"
              size="sm"
              variant="flat"
              onPress={() => setResetOpen(true)}
            >
              <RefreshCw className="w-4 h-4" />
            </Button>
          </div>
        </div>
        <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-5 gap-2">
          {SUB_FORMATS.map((f) => (
            <button
              key={f.key || "auto"}
              className="text-left rounded-xl border border-divider px-3 py-2 hover:bg-default-100 transition-colors disabled:opacity-50"
              disabled={!path}
              type="button"
              onClick={() => copyText(url(f.key), `${f.label} 订阅已复制`)}
            >
              <div className="flex items-center justify-between">
                <span className="text-sm font-medium">{f.label}</span>
                <Copy className="w-3.5 h-3.5 text-default-400" />
              </div>
              <div className="text-xs text-default-500">{f.hint}</div>
            </button>
          ))}
        </div>
        {showQr && path && (
          <div className="flex flex-col items-center gap-2 pt-1">
            <QrImage text={url("")} />
            <p className="text-xs text-default-500 font-mono break-all text-center">
              {url("")}
            </p>
          </div>
        )}
      </CardBody>

      <Modal
        backdrop="blur"
        classNames={{ base: modalBase }}
        isOpen={resetOpen}
        placement="center"
        onOpenChange={setResetOpen}
      >
        <ModalContent>
          {(close) => (
            <>
              <ModalHeader>
                <h2 className="text-lg font-bold">重置订阅地址</h2>
              </ModalHeader>
              <ModalBody>
                <p className="text-sm text-default-600">
                  旧的订阅地址会立即失效，所有客户端都需要重新导入新地址。
                </p>
              </ModalBody>
              <ModalFooter>
                <Button variant="light" onPress={close}>
                  取消
                </Button>
                <Button color="warning" isLoading={resetting} onPress={doReset}>
                  重置
                </Button>
              </ModalFooter>
            </>
          )}
        </ModalContent>
      </Modal>
    </Card>
  );
}
