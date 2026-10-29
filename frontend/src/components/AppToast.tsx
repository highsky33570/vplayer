type Props = {
  message: string | null;
};

/** Top-right flash notice (e.g. 登录成功). Parent controls show/hide. */
export function AppToast({ message }: Props) {
  if (!message) return null;
  return (
    <div className="app-toast" role="status" aria-live="polite">
      {message}
    </div>
  );
}
