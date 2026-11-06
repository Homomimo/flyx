import React, { useState, useEffect, useRef } from "react";
import { useNavigate, useLocation } from "react-router-dom";
import { toast } from "react-hot-toast";
import {
  ArrowDownToLine,
  ChevronsLeft,
  ChevronsRight,
  Globe,
  LayoutGrid,
  LineChart,
  Link2,
  LogOut,
  Menu,
  Rss,
  Server,
  Settings,
  Share2,
  UserRound,
  Users,
} from "lucide-react";

import { BrandLogo } from "@/components/brand-logo";
import { VersionFooter } from "@/components/version-footer";
import { getMonitorAccess } from "@/api";
import { safeLogout } from "@/utils/logout";
import { siteConfig } from "@/config/site";
import { useMobileBreakpoint } from "@/hooks/useMobileBreakpoint";
import { getAdminFlag } from "@/utils/session";
import { useHasMySubscriptions } from "@/hooks/use-has-my-subscriptions";

interface MenuItem {
  path: string;
  label: string;
  icon: React.ReactNode;
  adminOnly?: boolean;
  // Hidden for admins: the page lives under 中转 for them.
  userOnly?: boolean;
  adminLabel?: string;
  // Admin menu heading shown above this item.
  section?: string;
}

export default function AdminLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  const navigate = useNavigate();
  const location = useLocation();
  const [mobileMenuVisible, setMobileMenuVisible] = useState(false);
  const [isCollapsed, setIsCollapsed] = useState(
    () => localStorage.getItem("sidebar_collapsed") === "true",
  );
  const [isAdmin, setIsAdmin] = useState(() => getAdminFlag());
  const hasMySubs = useHasMySubscriptions(isAdmin);
  const [monitorAllowed, setMonitorAllowed] = useState<boolean | null>(null);
  const [monitorAccessReason, setMonitorAccessReason] = useState<string | null>(
    null,
  );
  const isMobile = useMobileBreakpoint();

  // 菜单项配置：管理员按 线路 / 分发 / 系统 分组（用户页含分组和限速），普通用户保持原样
  const menuItems: MenuItem[] = [
    {
      path: "/dashboard",
      label: "概览",
      icon: <LayoutGrid className="h-4 w-4" strokeWidth={1.75} />,
    },
    {
      path: "/forward",
      label: "规则",
      icon: <ArrowDownToLine className="h-4 w-4" strokeWidth={1.75} />,
      userOnly: true,
    },
    {
      path: "/my-subscriptions",
      label: "订阅",
      icon: <Rss className="h-4 w-4" strokeWidth={1.75} />,
      // Shown once the user has subscriptions assigned.
      userOnly: true,
    },
    {
      path: "/node",
      section: "线路",
      label: "服务器",
      icon: <Server className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/inbound",
      label: "节点",
      icon: <Globe className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/relay",
      label: "中转",
      icon: <Link2 className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/subscription",
      section: "分发",
      label: "订阅",
      icon: <Rss className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/user",
      label: "用户",
      icon: <Users className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/monitor",
      // Admins find monitoring under 节点 and 中转.
      userOnly: true,
      section: "系统",
      label: "监控",
      icon: <LineChart className="h-4 w-4" strokeWidth={1.75} />,
    },
    {
      path: "/panel-sharing",
      label: "共享",
      icon: <Share2 className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
    {
      path: "/config",
      label: "设置",
      icon: <Settings className="h-4 w-4" strokeWidth={1.75} />,
      adminOnly: true,
    },
  ];

  useEffect(() => {
    // 获取用户信息
    const adminFlag = getAdminFlag();

    setIsAdmin(adminFlag);

    // Monitor permission is not strictly role-based; non-admin users may be
    // granted access explicitly. Fetch a lightweight capability flag so we can
    // avoid a confusing 403 navigation.
    if (adminFlag) {
      setMonitorAllowed(true);
      setMonitorAccessReason(null);

      return;
    }

    let cancelled = false;

    (async () => {
      try {
        const res = await getMonitorAccess();

        if (cancelled) return;
        if (res.code === 0 && res.data) {
          setMonitorAllowed(Boolean(res.data.allowed));
          setMonitorAccessReason(
            res.data.allowed ? null : res.data.reason || null,
          );

          return;
        }
        // Fail open to preserve legacy navigation behavior.
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      } catch {
        if (cancelled) return;
        setMonitorAllowed(true);
        setMonitorAccessReason(null);
      }
    })();

    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    if (!isMobile) {
      setMobileMenuVisible(false);
    }
  }, [isMobile]);

  // The layout now stays mounted across pages: start each page at the top
  // and with the mobile menu closed, as a fresh layout did.
  const mainRef = useRef<HTMLElement>(null);

  useEffect(() => {
    mainRef.current?.scrollTo(0, 0);
    setMobileMenuVisible(false);
  }, [location.pathname]);

  // 退出登录
  const handleLogout = () => {
    safeLogout();
  };

  // 切换移动端菜单
  const toggleMobileMenu = () => {
    setMobileMenuVisible(!mobileMenuVisible);
  };

  // 隐藏移动端菜单
  const hideMobileMenu = () => {
    setMobileMenuVisible(false);
  };

  // 切换折叠状态
  const toggleCollapse = () => {
    const newCollapsed = !isCollapsed;

    setIsCollapsed(newCollapsed);
    localStorage.setItem("sidebar_collapsed", newCollapsed.toString());
  };

  // 菜单点击处理
  const handleMenuClick = (path: string) => {
    if (path === "/monitor" && monitorAllowed !== true) {
      if (monitorAllowed == null) {
        toast("正在检查监控权限，请稍后重试");

        return;
      }

      const hint =
        monitorAccessReason === "need_admin_grant"
          ? "暂无监控权限，请联系管理员在用户页面授予监控权限"
          : "暂无监控权限，请联系管理员授权";

      toast.error(hint);

      return;
    }

    navigate(path);
    if (isMobile) {
      hideMobileMenu();
    }
  };

  // 过滤菜单项（根据权限）
  const filteredMenuItems = menuItems
    .filter((item) => (isAdmin ? !item.userOnly : !item.adminOnly))
    .filter((item) => item.path !== "/my-subscriptions" || hasMySubs)
    .map((item) =>
      isAdmin && item.adminLabel ? { ...item, label: item.adminLabel } : item,
    );

  return (
    <div
      className={`flex bg-background ${isMobile ? "min-h-screen" : "h-screen overflow-hidden"}`}
    >
      {/* 移动端遮罩层 */}
      {isMobile && mobileMenuVisible && (
        <button
          aria-label="关闭菜单"
          className="fixed inset-0 z-40 bg-black/40"
          type="button"
          onClick={hideMobileMenu}
        />
      )}

      {/* 左侧菜单栏：贴边平铺，只用一条细线和内容区分开 */}
      <aside
        className={`
        ${isMobile ? "fixed top-0 left-0 h-screen" : "relative h-full"}
        ${isMobile && !mobileMenuVisible ? "-translate-x-full" : "translate-x-0"}
        ${isMobile ? "w-60" : isCollapsed ? "w-16" : "w-56"}
        z-50 flex flex-shrink-0 flex-col border-r border-divider bg-background
        transition-[width,transform] duration-200`}
      >
        {/* Logo 区域 */}
        <div
          className={`flex h-14 items-center overflow-hidden whitespace-nowrap ${isCollapsed ? "justify-center px-0" : "px-5"}`}
        >
          <BrandLogo className="h-6 w-6 flex-shrink-0" size={24} />
          {!isCollapsed && (
            <span className="ml-2.5 truncate text-[15px] font-semibold text-foreground">
              {siteConfig.name}
            </span>
          )}
        </div>

        {/* 菜单导航 */}
        <nav className="flex-1 overflow-y-auto overflow-x-hidden px-3 scrollbar-hide">
          <ul className="space-y-0.5">
            {filteredMenuItems.map((item) => {
              const isActive =
                location.pathname === item.path ||
                (item.path === "/relay" &&
                  isAdmin &&
                  ["/tunnel", "/forward"].includes(location.pathname)) ||
                (item.path === "/user" &&
                  ["/group", "/limit"].includes(location.pathname));
              const isMonitor = item.path === "/monitor";
              const isMonitorBlocked = isMonitor && monitorAllowed !== true;

              return (
                <li key={item.path}>
                  {isAdmin &&
                    item.section &&
                    (isCollapsed ? (
                      <div className="mx-2 my-2 border-t border-divider" />
                    ) : (
                      <div className="px-2 pb-1 pt-4 text-xs text-default-400">
                        {item.section}
                      </div>
                    ))}
                  <button
                    aria-current={isActive ? "page" : undefined}
                    aria-disabled={isMonitorBlocked}
                    className={`flex h-9 w-full items-center rounded-md text-left text-sm transition-colors ${isCollapsed ? "justify-center" : "gap-2.5 px-2"} ${
                      isActive
                        ? "bg-default-100 font-medium text-foreground"
                        : "text-default-500 hover:bg-default-50 hover:text-foreground"
                    } ${isMonitorBlocked ? "opacity-50" : ""}`}
                    title={
                      isCollapsed
                        ? isMonitorBlocked
                          ? `${item.label} (无权限)`
                          : item.label
                        : undefined
                    }
                    type="button"
                    onClick={() => handleMenuClick(item.path)}
                  >
                    <span className="flex h-4 w-4 flex-shrink-0 items-center justify-center">
                      {item.icon}
                    </span>
                    {!isCollapsed && (
                      <span className="truncate">{item.label}</span>
                    )}
                  </button>
                </li>
              );
            })}
          </ul>
        </nav>

        {/* 底部：账号、退出、版本和折叠 */}
        <div className="mt-auto flex flex-shrink-0 flex-col gap-1 overflow-hidden whitespace-nowrap px-3 py-3">
          {!isMobile &&
            [
              // Password, passkeys and GitHub live on the account page.
              {
                key: "account",
                label: "账户与登录",
                icon: UserRound,
                onPress: () => navigate("/profile"),
              },
              {
                key: "logout",
                label: "退出登录",
                icon: LogOut,
                onPress: handleLogout,
              },
            ].map((a) => (
              <button
                key={a.key}
                className={`flex h-8 items-center rounded-md text-sm text-default-500 transition-colors hover:bg-default-50 hover:text-foreground ${isCollapsed ? "justify-center" : "gap-2.5 px-2"}`}
                title={a.label}
                type="button"
                onClick={a.onPress}
              >
                <a.icon className="h-4 w-4 shrink-0" strokeWidth={1.75} />
                {!isCollapsed && <span>{a.label}</span>}
              </button>
            ))}
          <div
            className={`mt-2 flex items-center ${isCollapsed ? "justify-center" : "justify-between gap-2 pl-2"}`}
          >
            {!isCollapsed && (
              <VersionFooter
                containerClassName="min-w-0"
                poweredClassName="hidden"
                updateBadgeClassName="ml-1.5 text-foreground"
                version={siteConfig.version}
                versionClassName="truncate text-xs text-default-400"
              />
            )}
            {!isMobile && (
              <button
                aria-label={isCollapsed ? "展开菜单" : "收起菜单"}
                className="flex h-7 w-7 flex-shrink-0 items-center justify-center rounded-md text-default-400 transition-colors hover:bg-default-50 hover:text-foreground"
                type="button"
                onClick={toggleCollapse}
              >
                {isCollapsed ? (
                  <ChevronsRight className="h-4 w-4" />
                ) : (
                  <ChevronsLeft className="h-4 w-4" />
                )}
              </button>
            )}
          </div>
        </div>
      </aside>

      {/* 主内容区域 */}
      <div
        className={`relative flex flex-1 flex-col ${isMobile ? "min-h-0" : "h-full overflow-hidden"}`}
      >
        {isMobile && (
          <header className="flex h-12 items-center gap-2 border-b border-divider px-3">
            <button
              aria-label="打开菜单"
              className="flex h-8 w-8 items-center justify-center rounded-md text-foreground hover:bg-default-100"
              type="button"
              onClick={toggleMobileMenu}
            >
              <Menu className="h-5 w-5" strokeWidth={1.75} />
            </button>
            <BrandLogo className="h-5 w-5" size={20} />
            <span className="text-sm font-semibold">{siteConfig.name}</span>
          </header>
        )}

        <main ref={mainRef} className="flex-1 overflow-y-auto scrollbar-hide">
          <div className="mx-auto h-full w-full max-w-6xl">{children}</div>
        </main>
      </div>
    </div>
  );
}
