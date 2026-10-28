import { useEffect, useId, useRef, useState } from 'react';
import { createPortal } from 'react-dom';
import { useNavigate } from 'react-router-dom';
import { membershipModalActiveCopy } from '../account/accountLogic';
import { createMembershipOrder, listMembershipPlans, type MembershipPlan } from '../api';
import { useAuth } from '../auth/AuthContext';

function formatPrice(usd: number): string {
  if (Number.isInteger(usd)) return `$${usd}`;
  return `$${usd.toFixed(2)}`;
}

function planHint(days: number): string {
  if (days >= 365) return '全年畅看 · 最划算';
  if (days >= 180) return '半年畅看 · 超值';
  if (days >= 90) return '季度特惠';
  return '灵活体验';
}

type Props = {
  open: boolean;
  onClose: () => void;
};

export function VipPurchaseModal({ open, onClose }: Props) {
  const { user, token, ready, openLogin } = useAuth();
  const navigate = useNavigate();
  const titleId = useId();
  const closeRef = useRef<HTMLButtonElement>(null);
  const [plans, setPlans] = useState<MembershipPlan[]>([]);
  const [selected, setSelected] = useState('');
  const [loading, setLoading] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    if (!open) return;
    let cancelled = false;
    setLoading(true);
    setError('');
    listMembershipPlans()
      .then((rows) => {
        if (cancelled) return;
        setPlans(rows);
        if (rows.length > 0) {
          const preferred = rows.find((p) => p.duration_days >= 365)?.slug ?? rows[0].slug;
          setSelected((prev) => prev || preferred);
        }
      })
      .catch((e) => {
        if (!cancelled) setError(e instanceof Error ? e.message : '加载套餐失败');
      })
      .finally(() => {
        if (!cancelled) setLoading(false);
      });
    return () => {
      cancelled = true;
    };
  }, [open]);

  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') onClose();
    };
    window.addEventListener('keydown', onKey);
    closeRef.current?.focus();
    return () => {
      document.body.style.overflow = prev;
      window.removeEventListener('keydown', onKey);
    };
  }, [open, onClose]);

  if (!open || typeof document === 'undefined') return null;

  const isMember = Boolean(user?.is_member || user?.is_admin);
  const activeCopy = membershipModalActiveCopy(user);
  const selectedPlan = plans.find((p) => p.slug === selected) ?? null;

  const onPay = async () => {
    setError('');
    if (!ready) return;
    if (!user || !token) {
      onClose();
      openLogin('login');
      return;
    }
    if (!selected) {
      setError('请选择套餐');
      return;
    }
    setBusy(true);
    try {
      const order = await createMembershipOrder(token, selected);
      if (!order.payment_url) throw new Error('未返回支付链接');
      // Stay on our site (with 返回) — open GM Pay in a new tab when possible.
      // Popup blockers fall back to the result page CTA.
      window.open(order.payment_url, '_blank', 'noopener,noreferrer');
      onClose();
      navigate(`/vip/payment/result?order=${encodeURIComponent(order.order_no)}`);
    } catch (e) {
      setError(e instanceof Error ? e.message : '创建订单失败');
      setBusy(false);
    }
  };

  return createPortal(
    <div className="vip-modal-root" role="presentation">
      <button type="button" className="vip-modal-backdrop" aria-label="关闭" onClick={onClose} />
      <div
        className="vip-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
      >
        <div className="vip-modal-glow" aria-hidden />
        <header className="vip-modal-head">
          <div>
            <p className="vip-modal-kicker">Premium · USDT</p>
            <h2 id={titleId}>开通大会员</h2>
            <p className="vip-modal-lead">
              解锁 VIP蓝光影院专属片库。仅支持加密货币支付，到账后自动开通。
            </p>
          </div>
          <button
            ref={closeRef}
            type="button"
            className="vip-modal-close"
            aria-label="关闭弹窗"
            onClick={onClose}
          >
            ×
          </button>
        </header>

        {isMember && activeCopy ? <p className="vip-modal-active">{activeCopy}</p> : null}

        {loading ? <p className="vip-modal-status">正在加载套餐…</p> : null}
        {error ? (
          <p className="vip-modal-error" role="alert">
            {error}
          </p>
        ) : null}

        {!loading && plans.length > 0 ? (
          <div className="vip-modal-plans" role="radiogroup" aria-label="会员套餐">
            {plans.map((plan) => {
              const active = selected === plan.slug;
              return (
                <button
                  key={plan.slug}
                  type="button"
                  role="radio"
                  aria-checked={active}
                  className={`vip-modal-plan${active ? ' is-selected' : ''}`}
                  onClick={() => setSelected(plan.slug)}
                >
                  <span className="vip-modal-plan-top">
                    <span className="vip-modal-plan-name">{plan.name}</span>
                    {plan.duration_days >= 365 ? <span className="vip-modal-plan-tag">推荐</span> : null}
                  </span>
                  <strong className="vip-modal-plan-price">{formatPrice(plan.price_usd)}</strong>
                  <span className="vip-modal-plan-meta">
                    {plan.duration_days} 天 · {planHint(plan.duration_days)}
                  </span>
                </button>
              );
            })}
          </div>
        ) : null}

        <footer className="vip-modal-foot">
          <div className="vip-modal-summary">
            {selectedPlan ? (
              <>
                <span>已选</span>
                <strong>
                  {selectedPlan.name} · {formatPrice(selectedPlan.price_usd)}
                </strong>
              </>
            ) : (
              <span>请选择套餐</span>
            )}
          </div>
          <button
            type="button"
            className="vip-modal-pay"
            disabled={busy || loading || !selected}
            onClick={() => void onPay()}
          >
            {busy ? '正在跳转支付…' : user ? '去支付（USDT）' : '登录后开通'}
          </button>
          <p className="vip-modal-note">支付页将显示收款二维码与地址，确认后返回本站自动开通。</p>
        </footer>
      </div>
    </div>,
    document.body,
  );
}
