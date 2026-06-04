import * as React from "react";
import { usePageHeader } from "@/hooks/use-page-header";
import { useAuth } from "@/hooks/use-auth";
import { toast } from "sonner";
import {
  useUsers,
  useCreateUser,
  useDeleteUser,
  useSetUserRole,
  useResetUserPassword,
  ApiError,
  type ManagedUser,
  type UserRole,
} from "@/lib/users-api";
import { Button } from "@/components/ui/button";
import { Badge } from "@/components/ui/badge";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Loader2, Plus, Trash2, KeyRound, Users, ShieldAlert } from "lucide-react";

function errMessage(e: unknown, fallback: string): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof Error) return e.message;
  return fallback;
}

function RoleBadge({ role }: { role: UserRole }) {
  return (
    <Badge
      variant="outline"
      className={
        role === "admin"
          ? "border-amber-500/40 text-amber-600 dark:text-amber-400"
          : "text-muted-foreground"
      }
    >
      {role}
    </Badge>
  );
}

function CreateUserCard() {
  const create = useCreateUser();
  const [username, setUsername] = React.useState("");
  const [email, setEmail] = React.useState("");
  const [password, setPassword] = React.useState("");
  const [role, setRole] = React.useState<UserRole>("user");

  const canSubmit =
    username.trim().length > 0 && password.length >= 8 && !create.isPending;

  function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!canSubmit) return;
    create.mutate(
      { username: username.trim(), email: email.trim() || undefined, password, role },
      {
        onSuccess: () => {
          toast.success(`User "${username.trim()}" created`);
          setUsername("");
          setEmail("");
          setPassword("");
          setRole("user");
        },
        onError: (e) => toast.error(errMessage(e, "Failed to create user")),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-base">
          <Plus className="h-4 w-4" /> Add user
        </CardTitle>
        <CardDescription>
          Create a login that can sign in with username and password.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={submit}
          className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-5 lg:items-end"
        >
          <div className="space-y-1">
            <Label htmlFor="nu-username">Username</Label>
            <Input
              id="nu-username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              autoComplete="off"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="nu-email">Email (optional)</Label>
            <Input
              id="nu-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="off"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="nu-password">Password</Label>
            <Input
              id="nu-password"
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              autoComplete="new-password"
              placeholder="min 8 characters"
            />
          </div>
          <div className="space-y-1">
            <Label>Role</Label>
            <Select value={role} onValueChange={(v) => setRole(v as UserRole)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="user">user</SelectItem>
                <SelectItem value="admin">admin</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <Button type="submit" disabled={!canSubmit}>
            {create.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Create
          </Button>
        </form>
      </CardContent>
    </Card>
  );
}

function ResetPasswordDialog({
  user,
  onClose,
}: {
  user: ManagedUser | null;
  onClose: () => void;
}) {
  const reset = useResetUserPassword();
  const [password, setPassword] = React.useState("");

  React.useEffect(() => {
    setPassword("");
  }, [user]);

  function submit() {
    if (!user || password.length < 8) return;
    reset.mutate(
      { id: user.id, password },
      {
        onSuccess: () => {
          toast.success(`Password reset for "${user.username}"`);
          onClose();
        },
        onError: (e) => toast.error(errMessage(e, "Failed to reset password")),
      },
    );
  }

  return (
    <Dialog open={!!user} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reset password</DialogTitle>
          <DialogDescription>
            Set a new password for {user?.username}.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1">
          <Label htmlFor="rp-password">New password</Label>
          <Input
            id="rp-password"
            type="password"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="new-password"
            placeholder="min 8 characters"
          />
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            onClick={submit}
            disabled={password.length < 8 || reset.isPending}
          >
            {reset.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Reset password
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteUserDialog({
  user,
  onClose,
}: {
  user: ManagedUser | null;
  onClose: () => void;
}) {
  const del = useDeleteUser();

  function submit() {
    if (!user) return;
    del.mutate(user.id, {
      onSuccess: () => {
        toast.success(`User "${user.username}" deleted`);
        onClose();
      },
      onError: (e) => toast.error(errMessage(e, "Failed to delete user")),
    });
  }

  return (
    <Dialog open={!!user} onOpenChange={(o) => !o && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete user</DialogTitle>
          <DialogDescription>
            Permanently delete {user?.username}? This cannot be undone. Their API
            keys will also be removed.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant="outline" onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant="destructive"
            onClick={submit}
            disabled={del.isPending}
          >
            {del.isPending && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
            Delete
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function UsersTable({ currentUserId }: { currentUserId: number }) {
  const { data: users, isLoading } = useUsers();
  const setRole = useSetUserRole();
  const [resetTarget, setResetTarget] = React.useState<ManagedUser | null>(null);
  const [deleteTarget, setDeleteTarget] = React.useState<ManagedUser | null>(
    null,
  );

  if (isLoading) {
    return (
      <div className="flex items-center gap-2 p-6 text-sm text-muted-foreground">
        <Loader2 className="h-4 w-4 animate-spin" /> Loading users…
      </div>
    );
  }

  const rows = users ?? [];

  function changeRole(u: ManagedUser, role: UserRole) {
    if (role === u.role) return;
    setRole.mutate(
      { id: u.id, role },
      {
        onSuccess: () => toast.success(`${u.username} is now ${role}`),
        onError: (e) => toast.error(errMessage(e, "Failed to change role")),
      },
    );
  }

  return (
    <>
      <div className="rounded-md border">
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Username</TableHead>
              <TableHead>Email</TableHead>
              <TableHead>Role</TableHead>
              <TableHead className="text-right">Actions</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((u) => {
              const isSelf = u.id === currentUserId;
              const locked = u.protected || isSelf;
              return (
                <TableRow key={u.id}>
                  <TableCell className="font-medium">
                    <span className="flex items-center gap-2">
                      {u.username}
                      {u.protected && (
                        <Badge variant="secondary" className="text-[10px]">
                          primary admin
                        </Badge>
                      )}
                      {isSelf && !u.protected && (
                        <Badge variant="secondary" className="text-[10px]">
                          you
                        </Badge>
                      )}
                    </span>
                  </TableCell>
                  <TableCell className="text-muted-foreground">
                    {u.email || "—"}
                  </TableCell>
                  <TableCell>
                    {locked ? (
                      <RoleBadge role={u.role} />
                    ) : (
                      <Select
                        value={u.role}
                        onValueChange={(v) => changeRole(u, v as UserRole)}
                      >
                        <SelectTrigger className="h-8 w-28">
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value="user">user</SelectItem>
                          <SelectItem value="admin">admin</SelectItem>
                        </SelectContent>
                      </Select>
                    )}
                  </TableCell>
                  <TableCell className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={u.protected}
                        onClick={() => setResetTarget(u)}
                      >
                        <KeyRound className="mr-1 h-3.5 w-3.5" /> Password
                      </Button>
                      <Button
                        variant="outline"
                        size="sm"
                        className="text-destructive hover:text-destructive"
                        disabled={locked}
                        onClick={() => setDeleteTarget(u)}
                      >
                        <Trash2 className="h-3.5 w-3.5" />
                      </Button>
                    </div>
                  </TableCell>
                </TableRow>
              );
            })}
            {rows.length === 0 && (
              <TableRow>
                <TableCell
                  colSpan={4}
                  className="py-6 text-center text-sm text-muted-foreground"
                >
                  No users.
                </TableCell>
              </TableRow>
            )}
          </TableBody>
        </Table>
      </div>
      <ResetPasswordDialog
        user={resetTarget}
        onClose={() => setResetTarget(null)}
      />
      <DeleteUserDialog
        user={deleteTarget}
        onClose={() => setDeleteTarget(null)}
      />
    </>
  );
}

export function UsersPage() {
  const { setHeader } = usePageHeader();
  const { user } = useAuth();
  const isAdmin = user?.role === "admin";
  React.useEffect(() => setHeader({ title: "Users" }), [setHeader]);

  if (!isAdmin) {
    return (
      <div className="flex flex-col items-center justify-center gap-2 p-12 text-center text-muted-foreground">
        <ShieldAlert className="h-8 w-8" />
        <p>You need admin access to manage users.</p>
      </div>
    );
  }

  return (
    <div className="space-y-6 p-6">
      <section className="space-y-1">
        <h2 className="flex items-center gap-2 text-lg font-semibold">
          <Users className="h-5 w-5" /> User accounts
        </h2>
        <p className="text-sm text-muted-foreground">
          Create and manage logins. Non-admin users can submit requests; admins
          can approve them and manage the system.
        </p>
      </section>

      <CreateUserCard />

      <UsersTable currentUserId={user?.id ?? -1} />
    </div>
  );
}
