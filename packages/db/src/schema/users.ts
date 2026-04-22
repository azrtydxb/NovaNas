import { relations } from 'drizzle-orm';
import {
  index,
  pgTable,
  primaryKey,
  timestamp,
  uniqueIndex,
  uuid,
  varchar,
} from 'drizzle-orm/pg-core';

/**
 * Local projection of a Keycloak user. Keycloak is the source of truth for
 * credentials and group membership; this table exists so FKs from other
 * tables (sessions, audit, notifications, etc.) remain referentially sound
 * and so we can cache display fields without round-tripping to Keycloak.
 */
export const users = pgTable(
  'users',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    keycloakId: varchar('keycloak_id', { length: 128 }).notNull(),
    username: varchar('username', { length: 255 }).notNull(),
    email: varchar('email', { length: 320 }),
    displayName: varchar('display_name', { length: 255 }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    keycloakIdIdx: uniqueIndex('users_keycloak_id_idx').on(table.keycloakId),
    usernameIdx: uniqueIndex('users_username_idx').on(table.username),
  })
);

export const groups = pgTable(
  'groups',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    keycloakId: varchar('keycloak_id', { length: 128 }).notNull(),
    name: varchar('name', { length: 255 }).notNull(),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    keycloakIdIdx: uniqueIndex('groups_keycloak_id_idx').on(table.keycloakId),
    nameIdx: index('groups_name_idx').on(table.name),
  })
);

export const userGroups = pgTable(
  'user_groups',
  {
    userId: uuid('user_id')
      .notNull()
      .references(() => users.id, { onDelete: 'cascade' }),
    groupId: uuid('group_id')
      .notNull()
      .references(() => groups.id, { onDelete: 'cascade' }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    pk: primaryKey({ columns: [table.userId, table.groupId] }),
    userIdx: index('user_groups_user_idx').on(table.userId),
    groupIdx: index('user_groups_group_idx').on(table.groupId),
  })
);

export const usersRelations = relations(users, ({ many }) => ({
  groups: many(userGroups),
}));

export const groupsRelations = relations(groups, ({ many }) => ({
  members: many(userGroups),
}));

export const userGroupsRelations = relations(userGroups, ({ one }) => ({
  user: one(users, { fields: [userGroups.userId], references: [users.id] }),
  group: one(groups, { fields: [userGroups.groupId], references: [groups.id] }),
}));

export type User = typeof users.$inferSelect;
export type NewUser = typeof users.$inferInsert;
export type Group = typeof groups.$inferSelect;
export type NewGroup = typeof groups.$inferInsert;
export type UserGroup = typeof userGroups.$inferSelect;
export type NewUserGroup = typeof userGroups.$inferInsert;
